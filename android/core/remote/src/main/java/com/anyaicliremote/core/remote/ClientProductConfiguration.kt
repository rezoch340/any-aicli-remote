package com.anyaicliremote.core.remote

data class ClientProductConfiguration(
    val authorizationHeader: String,
    val clientName: String,
    val clientVersion: String,
) {
    init {
        require(authorizationHeader.isNotBlank()) { "authorizationHeader must not be blank" }
        require(clientName.isNotBlank()) { "clientName must not be blank" }
        require(clientVersion.isNotBlank()) { "clientVersion must not be blank" }
    }
}
