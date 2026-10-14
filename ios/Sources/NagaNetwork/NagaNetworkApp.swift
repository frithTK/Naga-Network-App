#if canImport(SwiftUI)
import SwiftUI
import NagaUI

@main
struct NagaNetworkApp: App {
    init() {
        NagaControl.start()
    }

    var body: some Scene {
        WindowGroup {
            RootView(dataDirectory: NagaControl.dataDirectory())
        }
    }
}
#endif
