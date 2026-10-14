#if canImport(SwiftUI)
import SwiftUI
import UIKit

struct DiagnosticsView: View {
    @ObservedObject var model: NagaAppModel
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        NagaPage("Диагностика", subtitle: "Состояние сервиса и журнал подключения.") {
            NagaCard {
                VStack(alignment: .leading, spacing: 16) {
                    Label(model.controlReachable == true ? "Сервис Naga доступен" : "Нет связи с сервисом Naga",
                          systemImage: model.controlReachable == true ? "checkmark.circle" : "exclamationmark.triangle")
                        .foregroundStyle(model.controlReachable == true ? NagaTheme.success : NagaTheme.warning)
                    Text(model.statusLabel).font(NagaTheme.font(18, weight: .semibold))
                    if model.health?.runtimeReady != true {
                        Text("VPN-ядро ещё не собрано для этого устройства.").foregroundStyle(NagaTheme.muted)
                    }
                    Button("Проверить соединение") { Task { await model.reload(); await model.loadDiagnostics() } }
                        .buttonStyle(NagaButtonStyle(primary: false))
                }
            }
            NagaCard {
                VStack(alignment: .leading, spacing: 16) {
                    HStack {
                        Text("Журнал").font(NagaTheme.font(18, weight: .semibold))
                        Spacer()
                        Button {
                            UIPasteboard.general.string = model.logs.map { "\($0.timestamp ?? "") [\($0.level)] \($0.component): \($0.message)" }.joined(separator: "\n")
                            model.notice = "Журнал скопирован."
                        } label: { Image(systemName: "doc.on.doc").frame(width: 44, height: 44) }
                        .accessibilityLabel("Скопировать журнал").disabled(model.logs.isEmpty)
                    }
                    if model.logs.isEmpty {
                        Text("Записей пока нет.").foregroundStyle(NagaTheme.muted)
                    } else {
                        ForEach(Array(model.logs.enumerated()), id: \.offset) { _, entry in
                            VStack(alignment: .leading, spacing: 5) {
                                Text("\(entry.level.uppercased()) · \(entry.component)").font(NagaTheme.font(10, weight: .semibold)).foregroundStyle(NagaTheme.tertiary)
                                Text(entry.message).font(NagaTheme.font(12)).textSelection(.enabled)
                                if let timestamp = entry.timestamp { Text(timestamp).font(NagaTheme.font(10)).foregroundStyle(NagaTheme.muted) }
                            }.frame(maxWidth: .infinity, alignment: .leading)
                            Divider().overlay(NagaTheme.line)
                        }
                    }
                }
            }
        }
        .task(id: scenePhase) {
            guard scenePhase == .active else { return }
            while !Task.isCancelled {
                await model.loadDiagnostics()
                do { try await Task.sleep(for: .seconds(2)) } catch { break }
            }
        }
    }
}
#endif
