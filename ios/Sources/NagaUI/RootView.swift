#if canImport(SwiftUI)
import SwiftUI
import NagaControlClient

enum NagaTab: String, CaseIterable, Identifiable {
    case home = "Главная", profiles = "Профили", servers = "Серверы", stats = "Статистика", settings = "Настройки"
    var id: String { rawValue }
    var icon: String {
        switch self {
        case .home: return "house.fill"
        case .profiles: return "rectangle.stack"
        case .servers: return "globe"
        case .stats: return "chart.bar"
        case .settings: return "gearshape"
        }
    }
}

public struct RootView: View {
    @StateObject private var model: NagaAppModel
    @Environment(\.scenePhase) private var scenePhase
    @State private var tab: NagaTab = .home
    @State private var showImport = false
    @State private var importText = ""
    @State private var showDiagnostics = false
    @ScaledMetric(relativeTo: .caption2) private var tabFontSize = 9.0
    private let dataDirectory: URL

    public init(dataDirectory: URL) {
        self.init(dataDirectory: dataDirectory, client: try! NagaControlClient())
    }

    public init(dataDirectory: URL, client: any NagaControling) {
        self.dataDirectory = dataDirectory
        _model = StateObject(wrappedValue: NagaAppModel(client: client))
    }

    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                Group {
                    switch tab {
                    case .home:
                        HomeView(model: model, onImport: openImport, onServers: { tab = .servers }, onDiagnostics: { showDiagnostics = true })
                    case .profiles: ProfilesView(model: model, onImport: openImport)
                    case .servers: ServersView(model: model)
                    case .stats: StatsView(model: model)
                    case .settings: SettingsView(model: model)
                    }
                }.frame(maxWidth: .infinity, maxHeight: .infinity)
            }
            .safeAreaInset(edge: .bottom, spacing: 0) { navigationBar }
            .navigationDestination(isPresented: $showDiagnostics) { DiagnosticsView(model: model) }
            .sheet(isPresented: $showImport) { ImportProfileView(model: model, initialText: importText) }
        }
        .safeAreaInset(edge: .top, spacing: 0) {
            if let message = model.lastError ?? model.notice { banner(message) }
        }
        .font(NagaTheme.font()).foregroundStyle(NagaTheme.text).tint(NagaTheme.accent)
        .preferredColorScheme(.dark)
        .task { if scenePhase == .active { model.activate(dataDirectory: dataDirectory) } }
        .onChange(of: scenePhase) { phase in
            if phase == .active { model.activate(dataDirectory: dataDirectory) }
            else { model.deactivate() }
        }
        .onOpenURL { url in
            guard let resolved = SubscriptionLink.resolve(url.absoluteString) else { return }
            openImport(resolved)
        }
    }

    private func openImport(_ text: String = "") {
        importText = text
        model.preview = nil
        showImport = true
    }

    private func banner(_ message: String) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: model.lastError == nil ? "checkmark.circle" : "exclamationmark.circle")
            Text(message).font(NagaTheme.font(12)).frame(maxWidth: .infinity, alignment: .leading)
            Button {
                model.lastError = nil
                model.notice = nil
            } label: { Image(systemName: "xmark").frame(width: 32, height: 32) }
            .accessibilityLabel("Закрыть сообщение")
        }.padding(.leading, 16).padding(.trailing, 8).padding(.vertical, 10)
            .foregroundStyle(model.lastError == nil ? NagaTheme.success : NagaTheme.warning)
            .background(NagaTheme.surface)
            .accessibilityElement(children: .contain)
    }

    private var navigationBar: some View {
        HStack(alignment: .top, spacing: 0) {
            ForEach(NagaTab.allCases) { item in
                Button { tab = item } label: {
                    VStack(spacing: 7) {
                        Image(systemName: item.icon).font(.system(size: 20, weight: .medium))
                        Text(item.rawValue).font(.custom("Inter-Medium", fixedSize: min(tabFontSize, 12)))
                            .lineLimit(2).multilineTextAlignment(.center)
                    }
                    .foregroundStyle(tab == item ? NagaTheme.accent : NagaTheme.tertiary)
                    .frame(maxWidth: .infinity, minHeight: 60)
                    .padding(.top, 10).padding(.horizontal, 1)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel(item.rawValue)
                .accessibilityAddTraits(tab == item ? .isSelected : [])
            }
        }.padding(.horizontal, 6).padding(.bottom, 6)
            .background(NagaTheme.surfaceLow)
            .overlay(alignment: .top) { Rectangle().fill(NagaTheme.line).frame(height: 1) }
    }
}
#endif
