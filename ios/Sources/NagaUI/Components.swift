#if canImport(SwiftUI)
import SwiftUI
import NagaControlClient

struct NagaCard<Content: View>: View {
    let content: Content
    init(@ViewBuilder content: () -> Content) { self.content = content() }
    var body: some View {
        content.padding(20).frame(maxWidth: .infinity, alignment: .leading)
            .background(NagaTheme.surfaceLow, in: RoundedRectangle(cornerRadius: 16))
            .overlay(RoundedRectangle(cornerRadius: 16).stroke(NagaTheme.line, lineWidth: 1))
    }
}

struct NagaPage<Content: View>: View {
    let title: String
    let subtitle: String
    let content: Content
    init(_ title: String, subtitle: String, @ViewBuilder content: () -> Content) {
        self.title = title
        self.subtitle = subtitle
        self.content = content()
    }
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 24) {
                VStack(alignment: .leading, spacing: 8) {
                    Text("NAGA NETWORK").font(NagaTheme.font(11, weight: .semibold)).tracking(2).foregroundStyle(NagaTheme.accent)
                    Text(title).font(NagaTheme.font(28, weight: .semibold, relativeTo: .title)).accessibilityAddTraits(.isHeader)
                    Text(subtitle).foregroundStyle(NagaTheme.muted).lineSpacing(4)
                }
                content
            }
            .padding(20).frame(maxWidth: 700, alignment: .leading).frame(maxWidth: .infinity)
        }
        .background(NagaTheme.background)
        .navigationBarTitleDisplayMode(.inline)
        .toolbarBackground(NagaTheme.background, for: .navigationBar)
    }
}

struct NagaButtonStyle: ButtonStyle {
    var primary = true
    func makeBody(configuration: Configuration) -> some View {
        configuration.label.font(NagaTheme.font(15, weight: .semibold))
            .frame(maxWidth: .infinity).padding(.vertical, 16).padding(.horizontal, 12)
            .foregroundStyle(primary ? NagaTheme.background : NagaTheme.text)
            .background(primary ? NagaTheme.accent : NagaTheme.surface, in: RoundedRectangle(cornerRadius: 14))
            .opacity(configuration.isPressed ? 0.75 : 1)
    }
}

struct NagaEmptyState: View {
    let icon: String
    let title: String
    let detail: String
    var body: some View {
        NagaCard {
            VStack(alignment: .leading, spacing: 12) {
                Image(systemName: icon).font(.system(size: 28)).foregroundStyle(NagaTheme.accent).accessibilityHidden(true)
                Text(title).font(NagaTheme.font(20, weight: .semibold))
                Text(detail).foregroundStyle(NagaTheme.muted).lineSpacing(4)
            }
        }
    }
}

struct NagaSettingRow: View {
    let icon: String
    let title: String
    let detail: String
    var body: some View {
        HStack(spacing: 14) {
            Image(systemName: icon).foregroundStyle(NagaTheme.accent).frame(width: 24).accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 5) {
                Text(title).font(NagaTheme.font(14, weight: .semibold)).foregroundStyle(NagaTheme.text)
                Text(detail).font(NagaTheme.font(12)).foregroundStyle(NagaTheme.muted)
            }
            Spacer(minLength: 4)
            Image(systemName: "chevron.right").font(.system(size: 12)).foregroundStyle(NagaTheme.tertiary).accessibilityHidden(true)
        }
        .padding(.vertical, 10).frame(minHeight: 52).contentShape(Rectangle())
    }
}

struct TrafficChart: View {
    let samples: [TrafficSample]
    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Трафик сессии").font(NagaTheme.font(17, weight: .semibold))
            if samples.count < 2 {
                Text("График появится во время подключения.").foregroundStyle(NagaTheme.muted).frame(maxWidth: .infinity, minHeight: 100)
            } else {
                Canvas { context, size in
                    for fraction in [0.25, 0.5, 0.75, 1.0] {
                        var grid = Path()
                        grid.move(to: CGPoint(x: 0, y: size.height * fraction))
                        grid.addLine(to: CGPoint(x: size.width, y: size.height * fraction))
                        context.stroke(grid, with: .color(NagaTheme.line), lineWidth: 1)
                    }
                    let maximum = Double(max(1, samples.map { max($0.upload, $0.download) }.max() ?? 1))
                    for upload in [false, true] {
                        var line = Path()
                        for (index, sample) in samples.enumerated() {
                            let value = Double(upload ? sample.upload : sample.download)
                            let point = CGPoint(x: size.width * Double(index) / Double(samples.count - 1),
                                                y: size.height * (1 - value / maximum))
                            if index == 0 { line.move(to: point) } else { line.addLine(to: point) }
                        }
                        context.stroke(line, with: .color(upload ? NagaTheme.muted : NagaTheme.accent), lineWidth: 2)
                    }
                }.frame(height: 130).accessibilityLabel("График скорости загрузки и отдачи")
            }
            HStack(spacing: 20) {
                Label("Загрузка", systemImage: "circle.fill").foregroundStyle(NagaTheme.accent)
                Label("Отдача", systemImage: "circle.fill").foregroundStyle(NagaTheme.muted)
            }.font(NagaTheme.font(11))
        }
    }
}
#endif
