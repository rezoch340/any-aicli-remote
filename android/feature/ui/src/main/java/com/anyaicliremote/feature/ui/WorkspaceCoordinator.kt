package com.anyaicliremote.feature.ui

import com.anyaicliremote.core.chat.AttachmentSelection
import com.anyaicliremote.core.model.ConnectionStatus
import com.anyaicliremote.core.model.SessionSummary
import com.anyaicliremote.core.model.WorkspaceFile

/** 工作区文件浏览与附件选择。 */
internal class WorkspaceCoordinator(
    private val scope: ChatOperationScope,
    private val workspaceBrowser: WorkspaceFileBrowser,
) {
    fun openFilePicker() {
        val session = scope.state.selectedSession ?: return
        if (scope.state.connection != ConnectionStatus.CONNECTED) return
        scope.update { it.copy(filePickerVisible = true, filePickerError = null) }
        loadWorkspaceFiles(session, ".")
    }

    fun closeFilePicker() {
        workspaceBrowser.cancel()
        scope.update { it.copy(filePickerVisible = false, filePickerLoading = false) }
    }

    fun browseWorkspace(path: String) {
        val session = scope.state.selectedSession ?: return
        if (!scope.state.filePickerVisible) return
        loadWorkspaceFiles(session, path)
    }

    fun toggleFileAttachment(file: WorkspaceFile) {
        if (file.directory) return
        scope.update { current ->
            val selection = AttachmentSelection(current.selectedFiles).toggle(file)
            current.copy(selectedFiles = selection.files)
        }
    }

    fun removeFileAttachment(path: String) {
        scope.update { current ->
            current.copy(selectedFiles = current.selectedFiles.filterNot { it.path == path })
        }
    }

    private fun loadWorkspaceFiles(session: SessionSummary, path: String) {
        val deviceId = scope.state.activeDeviceId ?: return
        val operationToken = scope.current()
        workspaceBrowser.load(
            WorkspaceLoadRequest(
                session = session,
                path = path,
                isCurrent = {
                    scope.isSessionCurrent(operationToken, deviceId, session.identity)
                },
                onLoading = {
                    scope.update {
                        it.copy(
                            filePickerPath = path,
                            filePickerLoading = true,
                            filePickerError = null,
                        )
                    }
                },
                onLoaded = { listing ->
                    scope.update {
                        it.copy(
                            filePickerPath = listing.path,
                            filePickerParent = listing.parent,
                            filePickerDirectories = listing.directories,
                            filePickerFiles = listing.files,
                            filePickerLoading = false,
                        )
                    }
                },
                onFailure = { message ->
                    scope.update {
                        it.copy(filePickerLoading = false, filePickerError = message)
                    }
                },
            ),
        )
    }
}
