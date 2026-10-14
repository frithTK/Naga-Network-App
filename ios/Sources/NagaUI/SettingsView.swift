#if canImport(SwiftUI)
import SwiftUI

struct SettingsView: View {
    @ObservedObject var model: NagaAppModel
    private var version: String { Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "—" }
    var body: some View {
        NagaPage("Настройки", subtitle: "Всё необходимое для вашего подключения.") {
            NagaCard {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Подключение").font(NagaTheme.font(18, weight: .semibold)).padding(.bottom, 6)
                    NavigationLink { ServerChoiceView(model: model) } label: {
                        NagaSettingRow(icon: "bolt", title: "Как выбирать сервер", detail: "Автоматически или вручную")
                    }
                    Divider().overlay(NagaTheme.line)
                    NavigationLink { DiagnosticsView(model: model) } label: {
                        NagaSettingRow(icon: "waveform.path.ecg", title: "Диагностика", detail: "Проверить соединение")
                    }
                }
            }
            NagaCard {
                VStack(alignment: .leading, spacing: 16) {
                    Text("Подписка").font(NagaTheme.font(18, weight: .semibold))
                    Toggle(isOn: $model.autoUpdate) {
                        VStack(alignment: .leading, spacing: 6) {
                            Text("Обновление профиля").font(NagaTheme.font(14, weight: .semibold))
                            Text(model.autoUpdate ? "По расписанию подписки" : "Вручную").font(NagaTheme.font(12)).foregroundStyle(NagaTheme.muted)
                        }
                    }.tint(NagaTheme.accent)
                }
            }
            NagaCard {
                VStack(alignment: .leading, spacing: 8) {
                    Text("О приложении").font(NagaTheme.font(18, weight: .semibold)).padding(.bottom, 6)
                    NavigationLink { HelpView() } label: {
                        NagaSettingRow(icon: "questionmark.circle", title: "Помощь", detail: "Ответы и подсказки")
                    }
                    Divider().overlay(NagaTheme.line)
                    versionRow("Naga Network", version)
                    versionRow("Ядро", coreVersion)
                    Text("GPL-3.0-or-later").font(NagaTheme.font(11)).foregroundStyle(NagaTheme.tertiary)
                }
            }
        }
    }

    private var coreVersion: String {
        guard let version = model.health?.version, !version.isEmpty else { return "Ядро ещё не собрано" }
        return version
    }

    private func versionRow(_ title: String, _ value: String) -> some View {
        HStack(alignment: .top) {
            Text(title)
            Spacer(minLength: 12)
            Text(value).foregroundStyle(NagaTheme.muted).multilineTextAlignment(.trailing)
        }.font(NagaTheme.font(12)).padding(.vertical, 10)
    }
}

struct ServerChoiceView: View {
    @ObservedObject var model: NagaAppModel
    private let choices = [
        ("auto", "Автоматически"),
        ("manual_fallback", "Ручной выбор с запасным сервером"),
        ("manual_strict", "Только выбранный сервер"),
    ]
    var body: some View {
        NagaPage("Как выбирать сервер", subtitle: "Авто, запасной вариант или только выбранная страна.") {
            NagaCard {
                VStack(alignment: .leading, spacing: 16) {
                    ForEach(choices, id: \.0) { mode, title in
                        Button { Task { await model.savePolicy(mode: mode) } } label: {
                            HStack(spacing: 12) {
                                Text(title).foregroundStyle(NagaTheme.text).multilineTextAlignment(.leading)
                                Spacer(minLength: 4)
                                Image(systemName: (model.policy?.mode ?? "auto") == mode ? "checkmark.circle.fill" : "circle")
                                    .foregroundStyle((model.policy?.mode ?? "auto") == mode ? NagaTheme.accent : NagaTheme.muted)
                            }.frame(minHeight: 44).contentShape(Rectangle())
                        }.buttonStyle(.plain).disabled(model.busy)
                    }
                    Divider().overlay(NagaTheme.line)
                    Text("На сотовой сети сначала пробуем VLESS. TUIC — последний запасной вариант.")
                        .font(NagaTheme.font(12)).foregroundStyle(NagaTheme.muted)
                }
            }
        }
    }
}
#endif
