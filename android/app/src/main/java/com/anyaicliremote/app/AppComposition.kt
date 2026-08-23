package com.anyaicliremote.app

import android.app.Application
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import com.anyaicliremote.core.remote.AnyAICLIRemoteClient
import com.anyaicliremote.core.remote.ClientRuntimeConfiguration
import com.anyaicliremote.core.remote.DeviceHealthProbe
import com.anyaicliremote.core.storage.DeviceProfileController
import com.anyaicliremote.core.storage.SecureProfileStore
import com.anyaicliremote.feature.ui.ChatViewModel

internal class AppComposition(private val application: Application) {
    val viewModelFactory: ViewModelProvider.Factory = object : ViewModelProvider.Factory {
        override fun <ViewModelType : ViewModel> create(modelClass: Class<ViewModelType>): ViewModelType {
            require(modelClass.isAssignableFrom(ChatViewModel::class.java))
            val configuration = ClientRuntimeConfiguration.Default
            val productConfiguration = ProductIdentifiers.clientConfiguration
            val client = AnyAICLIRemoteClient(productConfiguration, configuration)
            val probe = DeviceHealthProbe(configuration)
            val storeInitialization = runCatching {
                SecureProfileStore(application, ProductIdentifiers.profileStorageConfiguration)
            }
            val store = storeInitialization.getOrNull()
            val initialDevices = store?.let { runCatching { it.loadDevices() } }
            val initialError = storeInitialization.exceptionOrNull()?.message
                ?: initialDevices?.exceptionOrNull()?.message
                ?: if (store?.recoveredCorruptedStorage == true) {
                    "已清理无法读取的旧配对信息，请重新配对设备"
                } else null
            val controller = store?.let(::DeviceProfileController)
            return ChatViewModel(client, probe, controller, initialDevices?.getOrNull().orEmpty(), initialError, configuration) as ViewModelType
        }
    }
}
