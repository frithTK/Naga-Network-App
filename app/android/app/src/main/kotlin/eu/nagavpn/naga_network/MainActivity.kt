package eu.nagavpn.naga_network

import android.Manifest
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.net.Uri
import android.net.VpnService
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.Bundle
import android.os.ResultReceiver
import android.provider.Settings
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import java.io.File

class MainActivity : FlutterActivity() {
    private val updateChannel = "eu.nagavpn.naga_network/update"
    private val vpnChannel = "eu.nagavpn.naga_network/vpn"
    private var vpnPrepareResult: MethodChannel.Result? = null
    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        startControlPlane(application)
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
                    "installStatus" -> {
                        val prefs = getSharedPreferences(InstallStatusReceiver.PREFS, MODE_PRIVATE)
                        val status = prefs.getString("status", "idle")
                        result.success(mapOf("status" to status, "message" to prefs.getString("message", "")))
                        if (status == "installed" || status == "failed") {
                            prefs.edit().putString("status", "idle").putString("message", "").apply()
                        }
                    }
                    "dataDir" -> result.success(filesDir.absolutePath)
                    else -> result.notImplemented()
                }
            }
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, vpnChannel)
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "prepare" -> prepareVpn(result)
                    "start" -> {
                        requestNotifications()
                        startVpn(
                            call.argument<String>("engine") ?: NagaVpnService.ENGINE_SINGBOX,
                            call.argument<String>("routing") ?: "all_vpn",
                            call.argument<List<String>>("packages") ?: emptyList(),
                            result,
                        )
                    }
                    "stop" -> {
                        stopService(Intent(this, NagaVpnService::class.java).setAction(NagaVpnService.ACTION_DISCONNECT))
                        result.success(true)
                    }
                    "listPackages" -> result.success(listPackages())
                    else -> result.notImplemented()
                }
            }
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == VPN_PREPARE) {
            vpnPrepareResult?.success(resultCode == RESULT_OK)
            vpnPrepareResult = null
        }
    }

    private fun prepareVpn(result: MethodChannel.Result) {
        requestNotifications()
        val intent = VpnService.prepare(this)
        if (intent == null) {
            result.success(true)
            return
        }
        vpnPrepareResult = result
        @Suppress("DEPRECATION")
        startActivityForResult(intent, VPN_PREPARE)
    }

    private fun requestNotifications() {
        if (Build.VERSION.SDK_INT < 33) return
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) ==
            PackageManager.PERMISSION_GRANTED
        ) {
            return
        }
        ActivityCompat.requestPermissions(
            this,
            arrayOf(Manifest.permission.POST_NOTIFICATIONS),
            NOTIFICATION_REQUEST,
        )
    }

    private fun startVpn(engine: String, routing: String, packages: List<String>, result: MethodChannel.Result) {
        val handler = Handler(Looper.getMainLooper())
        var completed = false
        val timeout = Runnable {
            if (!completed) {
                completed = true
                stopService(Intent(this, NagaVpnService::class.java))
                result.error("vpn_start", "VPN-интерфейс не был создан вовремя.", null)
            }
        }
        val receiver = object : ResultReceiver(handler) {
            override fun onReceiveResult(resultCode: Int, resultData: Bundle?) {
                if (completed) return
                completed = true
                handler.removeCallbacks(timeout)
                result.success(resultCode == NagaVpnService.RESULT_READY)
            }
        }
        val intent =
            Intent(this, NagaVpnService::class.java)
                .putExtra(NagaVpnService.EXTRA_ENGINE, engine)
                .putExtra(NagaVpnService.EXTRA_ROUTING, routing)
                .putStringArrayListExtra(NagaVpnService.EXTRA_PACKAGES, ArrayList(packages))
                .putExtra(NagaVpnService.EXTRA_READY_RECEIVER, receiver)
        try {
            ContextCompat.startForegroundService(this, intent)
            handler.postDelayed(timeout, 10000)
        } catch (error: Exception) {
            completed = true
            result.error("vpn_start", error.message ?: "Не удалось запустить VPN-сервис.", null)
        }
    }

    private fun listPackages(): List<Map<String, String>> {
        val pm = packageManager
        val apps =
            if (Build.VERSION.SDK_INT >= 33) {
                pm.getInstalledApplications(PackageManager.ApplicationInfoFlags.of(0))
            } else {
                @Suppress("DEPRECATION")
                pm.getInstalledApplications(0)
            }
        return apps
            .asSequence()
            .filter { info ->
                info.packageName != packageName &&
                    (info.flags and ApplicationInfo.FLAG_SYSTEM) == 0 &&
                    pm.getLaunchIntentForPackage(info.packageName) != null
            }
            .map { info ->
                mapOf(
                    "name" to info.loadLabel(pm).toString(),
                    "process" to info.packageName,
                    "process_path" to info.packageName,
                )
            }
            .sortedBy { it["name"]?.lowercase() }
            .toList()
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
        var sessionId: Int? = null
        try {
            val info = if (Build.VERSION.SDK_INT >= 33) {
                packageManager.getPackageArchiveInfo(path, PackageManager.PackageInfoFlags.of(0))
            } else {
                @Suppress("DEPRECATION")
                packageManager.getPackageArchiveInfo(path, 0)
            }
            if (info?.packageName != packageName) {
                result.error("package", "APK не является обновлением Naga Network.", null)
                return
            }
            val installer = packageManager.packageInstaller
            val params =
                android.content.pm.PackageInstaller.SessionParams(
                    android.content.pm.PackageInstaller.SessionParams.MODE_FULL_INSTALL,
                )
            val id = installer.createSession(params)
            sessionId = id
            val nonce = java.util.UUID.randomUUID().toString()
            val prefs = getSharedPreferences(InstallStatusReceiver.PREFS, MODE_PRIVATE)
            if (!prefs.edit().putInt("session", id).putString("nonce", nonce).putString("status", "submitted").putString("message", "").commit()) {
                throw IllegalStateException("Не удалось сохранить состояние установки.")
            }
            installer.openSession(id).use { session ->
                session.openWrite("naga", 0, apk.length()).use { out ->
                    apk.inputStream().use { input -> input.copyTo(out) }
                    session.fsync(out)
                }
                val status = Intent(this, InstallStatusReceiver::class.java)
                    .setAction(InstallStatusReceiver.ACTION)
                    .putExtra("nonce", nonce)
                var flags = PendingIntent.FLAG_UPDATE_CURRENT
                if (Build.VERSION.SDK_INT >= 31) {
                    flags = flags or PendingIntent.FLAG_MUTABLE
                }
                val pending = PendingIntent.getBroadcast(this, id, status, flags)
                session.commit(pending.intentSender)
            }
            result.success(null)
        } catch (error: Exception) {
            sessionId?.let { runCatching { packageManager.packageInstaller.abandonSession(it) } }
            getSharedPreferences(InstallStatusReceiver.PREFS, MODE_PRIVATE).edit()
                .putString("status", "failed").putString("message", error.message ?: "PackageInstaller failed").apply()
            result.error("install", error.message ?: "PackageInstaller failed", null)
        }
    }

    companion object {
        private const val VPN_PREPARE = 9911
        private const val NOTIFICATION_REQUEST = 9912
    }
}
