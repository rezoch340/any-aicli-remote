package com.anyaicliremote.feature.ui

import kotlinx.coroutines.CancellationException

/** Provides one cancellation-safe boundary for asynchronous UI operations. */
internal object UiOperationRunner {
    @Suppress("TooGenericExceptionCaught") // A shared boundary must preserve all transport failures.
    suspend fun run(
        isCurrent: () -> Boolean = { true },
        onFailure: (Throwable) -> Unit,
        operation: suspend () -> Unit,
    ) {
        try {
            operation()
        } catch (exception: CancellationException) {
            throw exception
        } catch (exception: Exception) {
            if (isCurrent()) onFailure(exception)
        }
    }

    @Suppress("TooGenericExceptionCaught") // A shared boundary must preserve all storage failures.
    fun runSynchronously(onFailure: (Throwable) -> Unit, operation: () -> Unit) {
        try {
            operation()
        } catch (exception: Exception) {
            onFailure(exception)
        }
    }
}
