package eu.nagavpn.naga_network

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.system.OsConstants
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat

class NagaVpnService : VpnService() {
    private var tun: ParcelFileDescriptor? = null

    override fun onCreate() {
        super.onCreate()
        startControlPlane(application)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_DISCONNECT) {
            teardown()
            stopSelf()
            return START_NOT_STICKY
        }
        startForegroundNotification()
        if (!establish(intent)) {
            teardown()
            stopSelf()
            return START_NOT_STICKY
        }
        return START_STICKY
    }

    override fun onRevoke() {
        teardown()
        super.onRevoke()
    }

    override fun onDestroy() {
        teardown()
        super.onDestroy()
    }

    private fun startForegroundNotification() {
        val manager = getSystemService(NotificationManager::class.java)
        if (Build.VERSION.SDK_INT >= 26) {
            manager.createNotificationChannel(
                NotificationChannel(
                    CHANNEL_ID,
                    "Naga Network",
                    NotificationManager.IMPORTANCE_LOW,
                ),
            )
        }
        val disconnect =
            PendingIntent.getService(
                this,
                1,
                Intent(this, NagaVpnService::class.java).setAction(ACTION_DISCONNECT),
                pendingFlags(),
            )
        val open =
            PendingIntent.getActivity(
                this,
                0,
                Intent(this, MainActivity::class.java),
                pendingFlags(),
            )
        val notification: Notification =
            NotificationCompat.Builder(this, CHANNEL_ID)
                .setSmallIcon(R.drawable.ic_stat_naga)
                .setContentTitle("Naga Network")
                .setContentText("VPN подключён")
                .setContentIntent(open)
                .addAction(0, "Отключить", disconnect)
                .setOngoing(true)
                .build()
        if (Build.VERSION.SDK_INT >= 34) {
            ServiceCompat.startForeground(
                this,
                NOTIFICATION_ID,
                notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE,
            )
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }
    }

    private fun establish(intent: Intent?): Boolean {
        val engine = intent?.getStringExtra(EXTRA_ENGINE) ?: ENGINE_SINGBOX
        val routing = intent?.getStringExtra(EXTRA_ROUTING) ?: "all_vpn"
        val packages = intent?.getStringArrayListExtra(EXTRA_PACKAGES) ?: arrayListOf()
        val ipv4 = if (engine == ENGINE_OLC) OlcIPv4 else SingBoxIPv4
        val prefix = if (engine == ENGINE_OLC) OlcPrefix else SingBoxPrefix
        val builder =
            Builder()
                .setSession("Naga Network")
                .setMtu(MTU)
                .addAddress(ipv4, prefix)
                .addDnsServer(DNS)
                .setMetered(false)
                .setConfigureIntent(
                    PendingIntent.getActivity(
                        this,
                        0,
                        Intent(this, MainActivity::class.java),
                        pendingFlags(),
                    ),
                )
        builder.allowFamily(OsConstants.AF_INET)
        if (routing != "all_direct") {
            builder.addRoute("0.0.0.0", 0)
        }
        if (!applySplit(builder, routing, packages)) {
            return false
        }
        val established =
            try {
                builder.establish()
            } catch (_: Exception) {
                null
            }
        if (established == null) {
            return false
        }
        tun = established
        val fd = established.detachFd()
        tun = null
        val err = NagaControl.setTunFd(fd)
        if (err.isNotEmpty()) {
            runCatching { ParcelFileDescriptor.adoptFd(fd).close() }
            return false
        }
        return true
    }

    private fun applySplit(
        builder: Builder,
        routing: String,
        packages: List<String>,
    ): Boolean {
        return try {
            when (routing) {
                "selected_vpn" -> {
                    for (pkg in packages) {
                        if (pkg.isBlank() || pkg == packageName) continue
                        builder.addAllowedApplication(pkg)
                    }
                }
                "selected_direct" -> {
                    builder.addDisallowedApplication(packageName)
                    for (pkg in packages) {
                        if (pkg.isBlank() || pkg == packageName) continue
                        builder.addDisallowedApplication(pkg)
                    }
                }
                else -> builder.addDisallowedApplication(packageName)
            }
            true
        } catch (_: PackageManager.NameNotFoundException) {
            false
        }
    }

    private fun teardown() {
        NagaControl.stopRuntime()
        NagaControl.closeTun()
        tun?.close()
        tun = null
        stopForeground(STOP_FOREGROUND_REMOVE)
    }

    private fun pendingFlags(): Int {
        var flags = PendingIntent.FLAG_UPDATE_CURRENT
        if (Build.VERSION.SDK_INT >= 31) {
            flags = flags or PendingIntent.FLAG_IMMUTABLE
        }
        return flags
    }

    companion object {
        const val ACTION_DISCONNECT = "eu.nagavpn.naga_network.VPN_DISCONNECT"
        const val EXTRA_ENGINE = "engine"
        const val EXTRA_ROUTING = "routing"
        const val EXTRA_PACKAGES = "packages"
        const val ENGINE_SINGBOX = "singbox"
        const val ENGINE_OLC = "olcrtc"
        const val CHANNEL_ID = "naga.vpn"
        const val NOTIFICATION_ID = 24
        const val MTU = 1500
        const val DNS = "1.1.1.1"
        const val SingBoxIPv4 = "172.19.0.1"
        const val SingBoxPrefix = 30
        const val OlcIPv4 = "198.18.0.1"
        const val OlcPrefix = 32
    }
}
