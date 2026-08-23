package com.anyaicliremote.app.ui

import com.anyaicliremote.app.model.WorkspaceFile

data class AttachmentSelection(val files: List<WorkspaceFile> = emptyList()) {
    fun toggle(file: WorkspaceFile): AttachmentSelection =
        if (files.any { it.path == file.path }) {
            copy(files = files.filterNot { it.path == file.path })
        } else {
            copy(files = files + file)
        }

    fun clear(): AttachmentSelection = AttachmentSelection()
}
