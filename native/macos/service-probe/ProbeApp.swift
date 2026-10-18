import Combine
import Foundation
import ServiceManagement
import SwiftUI

@MainActor final class ProbeModel: ObservableObject {
  @Published var registration = ""
  @Published var output = "Пробник проверяет только регистрацию и состояние отдельной службы."
  private let service = SMAppService.daemon(plistName: probeHelperID + ".plist")

  init() { refresh() }

  func refresh() {
    switch service.status {
    case .notRegistered: registration = "Служба не зарегистрирована"
    case .enabled: registration = "Служба разрешена"
    case .requiresApproval: registration = "Нужно разрешение администратора в настройках macOS"
    case .notFound: registration = "macOS не находит встроенную службу"
    @unknown default: registration = "Неизвестное состояние службы"
    }
  }

  func register() {
    do {
      try service.register()
      output = "Регистрация запрошена. При необходимости разрешите службу в настройках macOS."
    } catch {
      show(error)
    }
    refresh()
  }

  func unregister() {
    do {
      try service.unregister()
      output = "Регистрация службы удалена. Обновите статус для проверки."
    } catch {
      show(error)
    }
    refresh()
  }

  private func show(_ error: Error) {
    let value = error as NSError
    output = "\(value.domain) / \(value.code): \(value.localizedDescription)"
  }

  func ping() {
    let connection = NSXPCConnection(machServiceName: probeHelperID, options: .privileged)
    connection.remoteObjectInterface = NSXPCInterface(with: NagaServiceProbeProtocol.self)
    connection.setCodeSigningRequirement("identifier \"\(probeHelperID)\"")
    output = "Ожидание ответа службы…"
    let deadline = DispatchWorkItem { [weak self] in
      self?.output = "Ответ не получен за 5 секунд. Проверьте разрешение и состояние службы."
      connection.invalidate()
    }
    DispatchQueue.main.asyncAfter(deadline: .now() + 5, execute: deadline)
    connection.resume()
    let remote = connection.remoteObjectProxyWithErrorHandler { [weak self] error in
      DispatchQueue.main.async {
        deadline.cancel()
        self?.show(error)
        connection.invalidate()
      }
    } as? NagaServiceProbeProtocol
    guard let remote else {
      deadline.cancel()
      connection.invalidate()
      output = "Не удалось получить XPC interface службы."
      return
    }
    remote.status { [weak self] response in
      DispatchQueue.main.async {
        deadline.cancel()
        defer { connection.invalidate() }
        guard connection.effectiveUserIdentifier == 0 else {
          self?.output = "Ответил непривилегированный процесс: \(response)"
          return
        }
        let data = Data(response.utf8)
        let value = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any]
        let compatible = value?["protocol_version"] as? Int == probeProtocolVersion
        self?.output = compatible ? response : "Версия helper отличается от приложения \(probeBuild):\n\(response)"
      }
    }
  }
}

@main struct NagaServiceProbeApp: App {
  @StateObject private var model = ProbeModel()

  var body: some Scene {
    WindowGroup {
      VStack(alignment: .leading, spacing: 16) {
        Text("Naga Service Probe — \(probeBuild)").font(.title)
        Text("macOS 14+, ARM64, ad-hoc подпись. Диагностическая служба не меняет маршруты, DNS или VPN.")
        Text(model.registration).font(.headline)
        HStack {
          Button("Зарегистрировать") { model.register() }
          Button("Настройки macOS") { SMAppService.openSystemSettingsLoginItems() }
          Button("Обновить статус") { model.refresh() }
        }
        HStack {
          Button("Запросить состояние helper") { model.ping() }
          Button("Удалить регистрацию службы") { model.unregister() }
        }
        ScrollView {
          Text(model.output).font(.system(.body, design: .monospaced)).textSelection(.enabled)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        Text("После проверки удалите регистрацию службы перед удалением приложения.").font(.footnote)
      }.padding(24).frame(minWidth: 740, minHeight: 380)
    }
  }
}
