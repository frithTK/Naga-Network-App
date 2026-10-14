#if canImport(SwiftUI)
import SwiftUI
import UIKit
import NagaControlClient

struct ProfilesView: View {
    @ObservedObject var model: NagaAppModel
    let onImport: (String) -> Void
    @State private var deleting: StoredProfileSummary?

    var body: some View {
        NagaPage("Профили", subtitle: "Ваши подписки и доступ к VPN.") {
            Button("Импорт по ссылке") { onImport("") }.buttonStyle(NagaButtonStyle())
            if model.profiles.isEmpty {
                NagaEmptyState(icon: "rectangle.stack", title: "Профилей пока нет",
                               detail: "Добавьте HTTPS-ссылку подписки или выберите файл конфигурации (.json, .conf).")
                Button("Вставить из буфера") { onImport(UIPasteboard.general.string ?? "") }
                    .buttonStyle(NagaButtonStyle(primary: false))
                Button("Импорт из файла") { onImport("") }.buttonStyle(NagaButtonStyle(primary: false))
            } else {
                ForEach(model.profiles) { profile in
                    NagaCard {
                        VStack(alignment: .leading, spacing: 14) {
                            Button { model.selectProfile(profile) } label: {
                                HStack(alignment: .top, spacing: 12) {
                                    VStack(alignment: .leading, spacing: 6) {
                                        Text(profile.name).font(NagaTheme.font(19, weight: .semibold)).foregroundStyle(NagaTheme.text)
                                        Text(profile.providerName.isEmpty ? profile.engine : profile.providerName).foregroundStyle(NagaTheme.muted)
                                    }
                                    Spacer(minLength: 4)
                                    Image(systemName: model.selectedProfileId == profile.id ? "checkmark.circle.fill" : "circle")
                                        .foregroundStyle(model.selectedProfileId == profile.id ? NagaTheme.accent : NagaTheme.tertiary)
                                }.contentShape(Rectangle())
                            }.buttonStyle(.plain).accessibilityLabel("Выбрать профиль \(profile.name)")
                            VStack(alignment: .leading, spacing: 6) {
                                Text(profile.unlimited ? "Безлимит" : "\(NagaFormat.bytes(profile.usedBytes)) из \(NagaFormat.bytes(profile.totalBytes))")
                                Text(NagaFormat.expiry(profile.expireUtc))
                            }.font(NagaTheme.font(12)).foregroundStyle(profile.allowsConnection() ? NagaTheme.muted : NagaTheme.warning)
                            if !profile.allowsConnection() {
                                Text("Проверьте срок и лимит подписки").font(NagaTheme.font(12)).foregroundStyle(NagaTheme.warning)
                            }
                            Divider().overlay(NagaTheme.line)
                            ViewThatFits(in: .horizontal) {
                                HStack(spacing: 16) { actions(profile) }
                                VStack(alignment: .leading, spacing: 16) { actions(profile) }
                            }.font(NagaTheme.font(12, weight: .medium))
                        }
                    }
                    .overlay(RoundedRectangle(cornerRadius: 16).stroke(model.selectedProfileId == profile.id ? NagaTheme.accent : .clear, lineWidth: 1))
                }
            }
        }
        .refreshable { await model.reload() }
        .alert("Удалить профиль?", isPresented: Binding(get: { deleting != nil }, set: { if !$0 { deleting = nil } })) {
            Button("Отмена", role: .cancel) { deleting = nil }
            Button("Удалить", role: .destructive) {
                if let profile = deleting { Task { await model.deleteProfile(profile) } }
                deleting = nil
            }
        } message: {
            Text("«\(deleting?.name ?? "")» будет убран из списка.")
        }
    }

    @ViewBuilder private func actions(_ profile: StoredProfileSummary) -> some View {
        Button { Task { await model.refreshProfile(profile) } } label: { Label("Обновить", systemImage: "arrow.clockwise") }
            .disabled(model.busy || !profile.hasSourceUrl)
        Button {
            Task {
                do {
                    let share = try await model.client.shareProfile(profile.id)
                    UIPasteboard.general.string = share.url
                    model.notice = "Ссылка скопирована."
                } catch { model.show(error) }
            }
        } label: { Label("Скопировать", systemImage: "doc.on.doc") }.disabled(model.busy || !profile.hasSourceUrl)
        Button(role: .destructive) { deleting = profile } label: { Label("Удалить", systemImage: "trash") }
            .disabled(model.busy || model.runtime?.blocksProfileDelete == true)
    }
}
#endif
