#if canImport(SwiftUI)
import SwiftUI

struct HelpView: View {
    var body: some View {
        NagaPage("Помощь", subtitle: "Быстрые ответы и подсказки по подключению.") {
            NagaCard {
                VStack(alignment: .leading, spacing: 12) {
                    Text("VPN не работает?").font(NagaTheme.font(22, weight: .medium))
                    Text("Проверьте интернет, профиль и срок подписки.").foregroundStyle(NagaTheme.muted)
                }
            }
            NagaCard {
                VStack(spacing: 20) {
                    DisclosureGroup("Как добавить профиль?") {
                        Text("Открой «Профили», нажми «Импорт по ссылке» и вставь HTTPS-ссылку подписки или выбери файл конфига (.json, .conf). Уже добавленный профиль по ссылке можно «Скопировать».")
                            .foregroundStyle(NagaTheme.muted).lineSpacing(4).padding(.top, 12)
                    }
                    Divider().overlay(NagaTheme.line)
                    DisclosureGroup("Почему VPN не подключается?") {
                        Text("Проверь интернет, обнови профиль и попробуй другой сервер. Если ошибка повторяется, открой диагностику.")
                            .foregroundStyle(NagaTheme.muted).lineSpacing(4).padding(.top, 12)
                    }
                }
            }
        }
    }
}
#endif
