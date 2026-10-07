package eu.nagavpn.naga_network

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageInstaller
import android.net.Uri
import android.os.Build
import android.provider.Settings
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import java.io.File

class MainActivity : FlutterActivity() {
    private val updateChannel = "eu.nagavpn.naga_network/update"
    private val installReceiver =
        object : BroadcastReceiver() {
            override fun onReceive(context: Context, intent: Intent) {}
        }

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, updateChannel)
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "applyAndroid" -> {
                        val path = call.argument<String>("apkPath")
                        if (path.isNullOrBlank()) {
                            result.error("path", "apkPath is required", null)
                        } else {
                            installApk(path, result)
                        }
                    }
                    "dataDir" -> result.success(filesDir.absolutePath)
                    else -> result.notImplemented()
                }
            }
    }

    override fun onStart() {
        super.onStart()
        val filter = IntentFilter(ACTION_INSTALL_COMPLETE)
        if (Build.VERSION.SDK_INT >= 33) {
            registerReceiver(installReceiver, filter, RECEIVER_NOT_EXPORTED)
        } else {
            @Suppress("UnspecifiedRegisterReceiverFlag")
            registerReceiver(installReceiver, filter)
        }
    }

    override fun onStop() {
        runCatching { unregisterReceiver(installReceiver) }
        super.onStop()
    }

    private fun installApk(path: String, result: MethodChannel.Result) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O &&
            !packageManager.canRequestPackageInstalls()
        ) {
            startActivity(
                Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES)
                    .setData(Uri.parse("package:$packageName")),
            )
            result.error(
                "permission",
                "Разрешите установку неизвестных приложений для Naga Network и повторите.",
                null,
            )
            return
        }
        val apk = File(path)
        if (!apk.isFile) {
            result.error("file", "APK не найден.", null)
            return
        }
        try {
            val installer = packageManager.packageInstaller
            val params =
                PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL)
            val sessionId = installer.createSession(params)
            installer.openSession(sessionId).use { session ->
                session.openWrite("naga", 0, apk.length()).use { out ->
                    apk.inputStream().use { input -> input.copyTo(out) }
                    session.fsync(out)
                }
                val status = Intent(ACTION_INSTALL_COMPLETE).setPackage(packageName)
                var flags = PendingIntent.FLAG_UPDATE_CURRENT
                if (Build.VERSION.SDK_INT >= 31) {
                    flags = flags or PendingIntent.FLAG_MUTABLE
                }
                val pending = PendingIntent.getBroadcast(this, sessionId, status, flags)
                session.commit(pending.intentSender)
            }
            result.success(null)
        } catch (error: Exception) {
            result.error("install", error.message ?: "PackageInstaller failed", null)
        }
    }

    companion object {
        private const val ACTION_INSTALL_COMPLETE = "eu.nagavpn.naga_network.INSTALL_COMPLETE"
    }
}
