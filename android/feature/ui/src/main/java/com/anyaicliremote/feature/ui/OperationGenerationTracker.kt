package com.anyaicliremote.feature.ui

internal data class OperationGeneration(
    val connection: Long,
    val session: Long,
)

internal class OperationGenerationTracker {
    private var connectionGeneration = 0L
    private var sessionGeneration = 0L

    fun current(): OperationGeneration = OperationGeneration(connectionGeneration, sessionGeneration)

    fun advanceConnection(): OperationGeneration {
        connectionGeneration += 1
        sessionGeneration += 1
        return current()
    }

    fun advanceSession(): OperationGeneration {
        sessionGeneration += 1
        return current()
    }

    fun isConnectionCurrent(operationGeneration: OperationGeneration): Boolean =
        operationGeneration.connection == connectionGeneration

    fun isSessionCurrent(operationGeneration: OperationGeneration): Boolean =
        isConnectionCurrent(operationGeneration) && operationGeneration.session == sessionGeneration
}
