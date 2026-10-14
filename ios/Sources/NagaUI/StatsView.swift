#if canImport(SwiftUI)
import SwiftUI
import NagaControlClient

struct StatsView: View {
    @ObservedObject var model: NagaAppModel
    var body: some View {
        NagaPage("Статистика", subtitle: "Трафик обновляется во время активного подключения.") {
            metric("Загрузка", icon: "arrow.down", bytes: model.runtime?.downloadBytes ?? 0,
                   rate: model.runtime?.downloadRateBytes ?? 0)
            metric("Отдача", icon: "arrow.up", bytes: model.runtime?.uploadBytes ?? 0,
                   rate: model.runtime?.uploadRateBytes ?? 0)
            NagaCard {
                VStack(alignment: .leading, spacing: 10) {
                    Label("Сессия", systemImage: "clock").foregroundStyle(NagaTheme.muted)
                    Text(NagaFormat.session(model.runtime?.sessionStartedAt)).font(NagaTheme.font(26, weight: .semibold)).monospacedDigit()
                    Text(model.connected ? "VPN подключён" : "Нет активного подключения").font(NagaTheme.font(12)).foregroundStyle(NagaTheme.muted)
                }
            }
            NagaCard { TrafficChart(samples: model.samples) }
            if let failover = model.runtime?.failoverMessage, !failover.isEmpty {
                Text(failover).foregroundStyle(NagaTheme.warning)
            }
        }
    }

    private func metric(_ title: String, icon: String, bytes: Int64, rate: Int64) -> some View {
        NagaCard {
            VStack(alignment: .leading, spacing: 10) {
                Label(title, systemImage: icon).foregroundStyle(NagaTheme.muted)
                Text(NagaFormat.bytes(bytes)).font(NagaTheme.font(26, weight: .semibold)).monospacedDigit()
                Text(model.runtime?.trafficAvailable == true ? "\(NagaFormat.bytes(rate))/с" : "Нет данных")
                    .font(NagaTheme.font(12)).foregroundStyle(NagaTheme.muted)
            }
        }
    }
}
#endif
