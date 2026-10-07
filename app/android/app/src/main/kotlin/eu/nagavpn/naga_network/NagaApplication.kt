package eu.nagavpn.naga_network

import android.app.Application

class NagaApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        startControlPlane(this)
    }
}

fun startControlPlane(app: Application) {
    runCatching {
        NagaControl.start(app.filesDir.absolutePath, app.applicationInfo.nativeLibraryDir)
    }
}
