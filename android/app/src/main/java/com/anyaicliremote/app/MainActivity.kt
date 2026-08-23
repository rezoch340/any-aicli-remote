package com.anyaicliremote.app

import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.widget.Toast

import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anyaicliremote.feature.ui.ChatViewModel
import com.anyaicliremote.app.ui.AnyAICLIRemoteRoot
import com.anyaicliremote.feature.ui.theme.AnyAICLIRemoteTheme
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning

class MainActivity : ComponentActivity() {
    private val appComposition by lazy { AppComposition(application) }
    private val viewModel: ChatViewModel by viewModels { appComposition.viewModelFactory }
    private val pairingCodeScannerOptions by lazy {
        GmsBarcodeScannerOptions.Builder()
            .setBarcodeFormats(Barcode.FORMAT_QR_CODE)
            .enableAutoZoom()
            .build()
    }
    private val pairingCodeScanner by lazy {
        GmsBarcodeScanning.getClient(this, pairingCodeScannerOptions)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        handlePairingIntent(intent)
        setContent {
            AnyAICLIRemoteTheme {
                val state by viewModel.state.collectAsStateWithLifecycle()
                AnyAICLIRemoteRoot(state, viewModel, onScanPairingCode = ::startPairingCodeScan)
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handlePairingIntent(intent)
    }

    private fun handlePairingIntent(intent: Intent?): Boolean {
        val data = intent?.data ?: return false
        if (!LegacyCompatibility.supportsPairingScheme(data.scheme) || data.host != "pair") return false
        val address = data.getQueryParameter("url") ?: return false
        val key = data.getQueryParameter("key").orEmpty()
        val name = data.getQueryParameter("name")
        intent.data = null
        viewModel.importPairing(address = address, key = key, name = name)
        return true
    }

    private fun startPairingCodeScan() {
        pairingCodeScanner.startScan()
            .addOnSuccessListener { barcode ->
                val rawValue = barcode.rawValue
                val valid = rawValue != null && handlePairingIntent(
                    Intent(Intent.ACTION_VIEW, Uri.parse(rawValue)),
                )
                if (!valid) {
                    Toast.makeText(this, "二维码无效，请扫描配对二维码", Toast.LENGTH_SHORT).show()
                }
            }
            .addOnCanceledListener { }
            .addOnFailureListener {
                Toast.makeText(this, "无法打开相机扫描，请稍后重试", Toast.LENGTH_SHORT).show()
            }
    }
}
