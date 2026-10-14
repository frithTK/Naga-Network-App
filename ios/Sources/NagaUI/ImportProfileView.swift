#if canImport(SwiftUI)
import SwiftUI
import UIKit
import UniformTypeIdentifiers
import NagaControlClient

struct ImportProfileView: View {
    @ObservedObject var model: NagaAppModel
    @Environment(\.dismiss) private var dismiss
    @State private var urlText: String
    @State private var showFilePicker = false
    @State private var fileConfig: String?
    @State private var fileName: String?
    @State private var errorText: String?

    init(model: NagaAppModel, initialText: String) {
        self.model = model
        _urlText = State(initialValue: initialText)
    }

    var body: some View {
        NavigationStack {
            NagaPage("Добавить профиль", subtitle: "Вставьте ссылку подписки или выберите файл конфигурации.") {
                NagaCard {
                    VStack(alignment: .leading, spacing: 16) {
                        Text("Ссылка подписки").font(NagaTheme.font(14, weight: .semibold))
                        TextField("HTTPS-ссылка подписки", text: $urlText, axis: .vertical)
                            .keyboardType(.URL).textInputAutocapitalization(.never).autocorrectionDisabled()
                            .lineLimit(2...5).padding(14)
                            .background(NagaTheme.surface, in: RoundedRectangle(cornerRadius: 10))
                            .accessibilityLabel("Ссылка подписки")
                            .onChange(of: urlText) { _ in
                                fileConfig = nil
                                fileName = nil
                                model.preview = nil
                                errorText = nil
                            }
                        HStack(spacing: 12) {
                            Button {
                                urlText = UIPasteboard.general.string ?? ""
                            } label: { Label("Вставить", systemImage: "doc.on.clipboard") }
                            Spacer()
                            Button { showFilePicker = true } label: { Label("Файл", systemImage: "doc") }
                        }.font(NagaTheme.font(13, weight: .medium)).frame(minHeight: 44)
                        if let fileName { Label(fileName, systemImage: "doc.text").foregroundStyle(NagaTheme.muted) }
                        if let errorText {
                            Text(errorText).foregroundStyle(NagaTheme.warning).font(NagaTheme.font(12))
                        }
                        if let error = model.lastError {
                            Text(error).foregroundStyle(NagaTheme.warning).font(NagaTheme.font(12))
                        }
                        Button {
                            Task { await checkPreview() }
                        } label: { Label("Проверить профиль", systemImage: "checkmark.shield") }
                        .buttonStyle(NagaButtonStyle(primary: false)).disabled(model.busy || !hasInput)
                    }
                }
                if let preview = model.preview {
                    NagaCard {
                        VStack(alignment: .leading, spacing: 10) {
                            Text(preview.profileTitle).font(NagaTheme.font(19, weight: .semibold))
                            if !preview.providerName.isEmpty { Text(preview.providerName).foregroundStyle(NagaTheme.muted) }
                            Text(preview.unlimited ? "Безлимит" : NagaFormat.bytes(preview.totalBytes)).foregroundStyle(NagaTheme.muted)
                            Text(NagaFormat.expiry(preview.expireUtc)).font(NagaTheme.font(12)).foregroundStyle(NagaTheme.muted)
                            if !preview.canConnect { Text("Проверьте срок и лимит подписки").foregroundStyle(NagaTheme.warning) }
                        }
                    }
                }
                Button {
                    Task { await save() }
                } label: {
                    HStack(spacing: 10) {
                        if model.busy { ProgressView().tint(NagaTheme.background) }
                        Text(model.busy ? "Добавляем…" : "Добавить профиль")
                    }
                }.buttonStyle(NagaButtonStyle()).disabled(model.busy || !hasInput)
                Text("Поддерживаются подписки, sing-box JSON и AmneziaWG .conf.")
                    .font(NagaTheme.font(12)).foregroundStyle(NagaTheme.muted)
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Закрыть") { dismiss() }.disabled(model.busy)
                }
            }
        }
        .font(NagaTheme.font()).foregroundStyle(NagaTheme.text).tint(NagaTheme.accent).preferredColorScheme(.dark)
        .interactiveDismissDisabled(model.busy)
        .fileImporter(isPresented: $showFilePicker, allowedContentTypes: [.json, .text, .data], allowsMultipleSelection: false) { result in
            do {
                guard let url = try result.get().first else { return }
                try readFile(url)
            } catch {
                errorText = "Не удалось прочитать файл. Выберите JSON или .conf."
            }
        }
    }

    private var hasInput: Bool { fileConfig != nil || !urlText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }

    private func readFile(_ url: URL) throws {
        let scoped = url.startAccessingSecurityScopedResource()
        defer { if scoped { url.stopAccessingSecurityScopedResource() } }
        guard ["json", "conf", "txt"].contains(url.pathExtension.lowercased()) else {
            errorText = "Выберите файл .json, .conf или .txt."
            return
        }
        let values = try url.resourceValues(forKeys: [.fileSizeKey, .isRegularFileKey])
        guard values.isRegularFile == true, (values.fileSize ?? 0) <= 16 * 1_024 * 1_024 else {
            errorText = "Файл слишком большой. Максимум — 16 МБ."
            return
        }
        let data = try Data(contentsOf: url)
        guard data.count <= 16 * 1_024 * 1_024, let config = String(data: data, encoding: .utf8), !config.isEmpty else {
            errorText = "Файл пустой или содержит неподдерживаемый текст."
            return
        }
        model.preview = nil
        model.lastError = nil
        errorText = nil
        fileConfig = config
        fileName = url.lastPathComponent
    }

    private func checkPreview() async {
        model.lastError = nil
        if let config = fileConfig {
            _ = await model.previewConfig(config, name: fileName)
        } else if let url = SubscriptionLink.resolve(urlText) {
            _ = await model.previewURL(url)
        } else { errorText = "Вставьте HTTPS-ссылку подписки." }
    }

    private func save() async {
        model.lastError = nil
        let success: Bool
        if let config = fileConfig { success = await model.importConfig(config, name: fileName) }
        else if let url = SubscriptionLink.resolve(urlText) { success = await model.importURL(url) }
        else { errorText = "Вставьте HTTPS-ссылку подписки."; return }
        if success { dismiss() }
    }
}
#endif
