package com.anyaicliremote.core.chat

import com.anyaicliremote.core.model.WorkspaceFile
import org.junit.Assert.assertEquals
import org.junit.Test

class AttachmentSelectionTest {
    private val firstFile = WorkspaceFile("first.txt", "/workspace/first.txt", "first.txt")
    private val secondFile = WorkspaceFile("second.txt", "/workspace/second.txt", "second.txt")

    @Test
    fun togglingSameFileRemovesItAndClearDropsAllFiles() {
        val selected = AttachmentSelection().toggle(firstFile).toggle(secondFile)
        assertEquals(listOf(firstFile, secondFile), selected.files)
        assertEquals(listOf(secondFile), selected.toggle(firstFile).files)
        assertEquals(emptyList<WorkspaceFile>(), selected.clear().files)
    }

    @Test
    fun samePathDoesNotCreateDuplicateSelection() {
        val duplicate = firstFile.copy(name = "renamed.txt")
        assertEquals(emptyList<WorkspaceFile>(), AttachmentSelection().toggle(firstFile).toggle(duplicate).files)
    }
}
