#if canImport(SwiftUI)
import SwiftUI
import NagaControlClient

struct ServersView: View {
    @ObservedObject var model: NagaAppModel
    private var countries: [String] { Set(model.nodes.map(\.country)).sorted() }

    var body: some View {
        NagaPage("Серверы", subtitle: model.connected ? "Выберите сервер или доверьте выбор Naga." : "Общий пул узлов из сохранённых профилей.") {
            if model.nodes.isEmpty {
                NagaEmptyState(icon: "globe", title: "Серверы пока не загружены", detail: "Добавьте профиль, чтобы увидеть доступные узлы.")
            } else {
                Button { Task { await model.chooseNode(nil) } } label: {
                    NagaCard {
                        HStack(spacing: 12) {
                            Image(systemName: "bolt.fill").foregroundStyle(NagaTheme.accent)
                            VStack(alignment: .leading, spacing: 5) {
                                Text("Автоматический выбор").font(NagaTheme.font(16, weight: .semibold)).foregroundStyle(NagaTheme.text)
                                Text("Naga выберет подходящий сервер.").font(NagaTheme.font(12)).foregroundStyle(NagaTheme.muted)
                            }
                            Spacer()
                            if model.selectedNodeId == nil { Image(systemName: "checkmark.circle.fill").foregroundStyle(NagaTheme.accent) }
                        }
                    }
                }.buttonStyle(.plain).disabled(model.busy)
                ForEach(countries, id: \.self) { country in
                    VStack(alignment: .leading, spacing: 12) {
                        Text(country.isEmpty ? "Другие серверы" : country).font(NagaTheme.font(14, weight: .semibold)).foregroundStyle(NagaTheme.muted)
                        ForEach(model.nodes.filter { $0.country == country }) { node in
                            Button { Task { await model.chooseNode(node) } } label: {
                                NagaCard {
                                    HStack(alignment: .top, spacing: 12) {
                                        VStack(alignment: .leading, spacing: 8) {
                                            Text(node.tag).font(NagaTheme.font(15, weight: .semibold)).foregroundStyle(NagaTheme.text)
                                            HStack {
                                                Text(node.protocolName)
                                                if let latency = node.latencyMs, latency > 0 { Text("\(latency) мс") }
                                                else { Text("Не измерено") }
                                            }.font(NagaTheme.font(12)).foregroundStyle(NagaTheme.muted)
                                        }
                                        Spacer(minLength: 4)
                                        if model.selectedNodeId == node.id {
                                            Image(systemName: "checkmark.circle.fill").foregroundStyle(NagaTheme.accent)
                                        }
                                    }
                                }
                            }.buttonStyle(.plain).disabled(model.busy)
                        }
                    }
                }
                Text("Задержка измеряется после подключения VPN.").font(NagaTheme.font(12)).foregroundStyle(NagaTheme.muted)
                Button("Обновить список") { Task { await model.reload() } }.buttonStyle(NagaButtonStyle(primary: false))
            }
        }.refreshable { await model.reload() }
    }
}
#endif
