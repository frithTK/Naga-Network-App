#if canImport(SwiftUI)
import SwiftUI
import UIKit
import NagaControlClient

struct HomeView: View {
    @ObservedObject var model: NagaAppModel
    let onImport: (String) -> Void
    let onServers: () -> Void
    let onDiagnostics: () -> Void

    private var statusColor: Color {
        if model.controlReachable == false || model.blockedProfile { return NagaTheme.warning }
        if model.connected { return NagaTheme.success }
        if model.runtime?.status == "failed" { return NagaTheme.error }
        return NagaTheme.muted
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                HStack(spacing: 10) {
                    if let mark = NagaAssets.image("naga-mark-red", directory: "Brand") {
                        mark.resizable().scaledToFit().frame(width: 30, height: 30).accessibilityHidden(true)
                    }
                    Text("Naga Network").font(NagaTheme.font(22, weight: .semibold, relativeTo: .title2))
                    Spacer()
                    Button(action: onDiagnostics) {
                        Image(systemName: "waveform.path.ecg").frame(width: 44, height: 44).foregroundStyle(NagaTheme.muted)
                    }.accessibilityLabel("Диагностика")
                }
                if model.controlReachable == false {
                    NagaCard {
                        VStack(alignment: .leading, spacing: 10) {
                            Label("Нет связи с сервисом Naga", systemImage: "exclamationmark.triangle").foregroundStyle(NagaTheme.warning)
                            Text("Состояние VPN неизвестно. Проверьте соединение в диагностике.").foregroundStyle(NagaTheme.muted)
                            Button("Открыть диагностику", action: onDiagnostics).buttonStyle(NagaButtonStyle(primary: false))
                        }
                    }
                }
                NagaCard {
                    VStack(alignment: .leading, spacing: 16) {
                        Text("СИСТЕМНЫЙ VPN").font(NagaTheme.font(10, weight: .semibold)).tracking(2).foregroundStyle(NagaTheme.tertiary)
                        HStack(alignment: .center, spacing: 12) {
                            VStack(alignment: .leading, spacing: 10) {
                                Circle().fill(statusColor).frame(width: 8, height: 8)
                                Text(model.statusLabel).font(NagaTheme.font(27, weight: .semibold, relativeTo: .title)).fixedSize(horizontal: false, vertical: true)
                                Text(model.connected ? "Ваше подключение защищено." : "Ваш безопасный доступ к сети.")
                                    .foregroundStyle(NagaTheme.muted)
                                    .fixedSize(horizontal: false, vertical: true)
                            }.frame(maxWidth: .infinity, alignment: .leading)
                            if let mascot = NagaAssets.image(model.mascot, directory: "Mascot") {
                                mascot.resizable().scaledToFit().frame(width: 105, height: 160).accessibilityHidden(true)
                            }
                        }
                        if model.blockedProfile {
                            Text("Обновите подписку, чтобы подключиться").foregroundStyle(NagaTheme.warning)
                            Text("Проверьте срок и лимит подписки").font(NagaTheme.font(12)).foregroundStyle(NagaTheme.muted)
                        }
                        if let failover = model.runtime?.failoverMessage, !failover.isEmpty {
                            Text(failover).font(NagaTheme.font(12)).foregroundStyle(NagaTheme.warning)
                        }
                        Button(action: model.connect) {
                            Label(model.connected ? "Отключить" : "Подключить", systemImage: "power")
                        }.buttonStyle(NagaButtonStyle()).disabled(model.busy || model.connectionBusy || model.blockedProfile)
                    }
                }
                if let profile = model.selectedProfile {
                    NagaCard {
                        VStack(alignment: .leading, spacing: 10) {
                            Text("ПРОФИЛЬ").font(NagaTheme.font(10, weight: .semibold)).tracking(2).foregroundStyle(NagaTheme.tertiary)
                            Text(profile.name).font(NagaTheme.font(18, weight: .semibold))
                            if !profile.providerName.isEmpty { Text(profile.providerName).foregroundStyle(NagaTheme.muted) }
                            HStack {
                                Text(profile.unlimited ? "Безлимит" : NagaFormat.bytes(profile.totalBytes))
                                Spacer()
                                Text(NagaFormat.expiry(profile.expireUtc)).multilineTextAlignment(.trailing)
                            }.font(NagaTheme.font(12)).foregroundStyle(NagaTheme.muted)
                        }
                    }
                    Button(action: onServers) {
                        NagaCard {
                            NagaSettingRow(icon: "globe", title: model.runtime?.activeNode ?? model.selectedNode?.tag ?? "Автоматический выбор",
                                           detail: model.runtime?.activeProtocol ?? model.selectedNode?.protocolName ?? "Выбрать сервер")
                        }
                    }.buttonStyle(.plain)
                    NagaCard { TrafficChart(samples: model.samples) }
                } else {
                    NagaCard {
                        VStack(alignment: .leading, spacing: 16) {
                            Text("Добавить профиль").font(NagaTheme.font(20, weight: .semibold))
                            Text("Добавьте подписку или файл конфигурации, чтобы подключиться.").foregroundStyle(NagaTheme.muted)
                            Button("Импорт по ссылке") { onImport("") }.buttonStyle(NagaButtonStyle())
                            Button("Вставить из буфера") { onImport(UIPasteboard.general.string ?? "") }.buttonStyle(NagaButtonStyle(primary: false))
                        }
                    }
                }
            }
            .padding(20).frame(maxWidth: 700).frame(maxWidth: .infinity)
        }.background(NagaTheme.background)
    }
}
#endif
