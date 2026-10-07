package eu.nagavpn.naga_network

object NagaControl {
    @Volatile
    private var loaded = false

    fun ensureLoaded(): Boolean {
        if (loaded) return true
        return runCatching {
            System.loadLibrary("naga")
            loaded = true
            true
        }.getOrDefault(false)
    }

    fun start(dataDir: String, nativeLibDir: String): String {
        if (!ensureLoaded()) {
            return "native library is missing"
        }
        return jniStart(dataDir, nativeLibDir)
    }

    fun stop() {
        if (ensureLoaded()) jniStop()
    }

    fun stopRuntime() {
        if (ensureLoaded()) jniStopRuntime()
    }

    fun setTunFd(fd: Int): String {
        if (!ensureLoaded()) {
            return "native library is missing"
        }
        return jniSetTunFd(fd)
    }

    fun closeTun() {
        if (ensureLoaded()) jniCloseTun()
    }

    @JvmStatic
    external fun jniStart(dataDir: String, nativeLibDir: String): String

    @JvmStatic
    external fun jniStop()

    @JvmStatic
    external fun jniStopRuntime()

    @JvmStatic
    external fun jniSetTunFd(fd: Int): String

    @JvmStatic
    external fun jniCloseTun()
}
