package com.anyaicliremote.feature.ui

import com.anyaicliremote.core.model.SessionSummary
import com.anyaicliremote.core.session.SessionController
import com.anyaicliremote.core.session.WorkspaceListing
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch

/** Serializes workspace listing requests and discards stale results at the caller boundary. */
internal class WorkspaceFileBrowser(
    private val scope: CoroutineScope,
    private val sessionController: SessionController,
) {
    private var browsingJob: Job? = null

    fun cancel() {
        browsingJob?.cancel()
        browsingJob = null
    }

    fun load(request: WorkspaceLoadRequest) {
        cancel()
        request.onLoading()
        browsingJob = scope.launch {
            UiOperationRunner.run(
                isCurrent = request.isCurrent,
                onFailure = { exception -> request.onFailure(exception.message ?: "无法读取工作区") },
            ) {
                val listing = sessionController.listWorkspace(request.session, request.path)
                if (!request.isCurrent()) return@run
                request.onLoaded(listing)
            }
        }
    }
}

data class WorkspaceLoadRequest(
    val session: SessionSummary,
    val path: String,
    val isCurrent: () -> Boolean,
    val onLoading: () -> Unit,
    val onLoaded: (WorkspaceListing) -> Unit,
    val onFailure: (String) -> Unit,
)
