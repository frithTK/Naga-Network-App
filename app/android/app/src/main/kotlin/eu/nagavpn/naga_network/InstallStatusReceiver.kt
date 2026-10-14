package eu.nagavpn.naga_network

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.os.Build

/** A manifest receiver survives Activity.onStop and PackageInstaller UI. */
class InstallStatusReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != ACTION) return
        val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        val session = intent.getIntExtra(PackageInstaller.EXTRA_SESSION_ID, -1)
        if (session < 0 || session != prefs.getInt("session", -1) ||
            intent.getStringExtra("nonce") != prefs.getString("nonce", null)) return
        val status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)
        when (status) {
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                prefs.edit().putString("status", "confirmation_required").apply()
                val confirmation = if (Build.VERSION.SDK_INT >= 33) {
                    intent.getParcelableExtra(Intent.EXTRA_INTENT, Intent::class.java)
                } else {
                    @Suppress("DEPRECATION")
                    intent.getParcelableExtra<Intent>(Intent.EXTRA_INTENT)
                }
                if (confirmation == null) {
                    fail(context, "Система не предоставила подтверждение установки.")
                    return
                }
                try {
                    context.startActivity(confirmation.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                } catch (_: Exception) {
                    fail(context, "Не удалось открыть подтверждение установки. Повторите обновление из приложения.")
                }
            }
            PackageInstaller.STATUS_SUCCESS -> prefs.edit()
                .putString("status", "installed").putString("message", "")
                .remove("session").remove("nonce").apply()
            else -> fail(context, "Установка отменена или завершилась ошибкой (код $status).")
        }
    }
    private fun fail(context: Context, message: String) {
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
            .putString("status", "failed").putString("message", message)
            .remove("session").remove("nonce").apply()
    }
    companion object {
        const val PREFS = "naga-install"
        const val ACTION = "eu.nagavpn.naga_network.INSTALL_COMPLETE"
    }
}
