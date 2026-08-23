package com.anyaicliremote.core.chat

import com.anyaicliremote.core.model.ChatBlock
import com.anyaicliremote.core.model.ChatBlockKind
import com.anyaicliremote.core.model.ToolRunState
import org.junit.Assert.assertEquals
import org.junit.Test

class TurnFinalizationTest {
    @Test
    fun finalizationEndsOnlyActiveToolBlocks() {
        val blocks = listOf(
            ChatBlock("pending", ChatBlockKind.TOOL, toolState = ToolRunState.PENDING),
            ChatBlock("running", ChatBlockKind.TOOL, toolState = ToolRunState.RUNNING),
            ChatBlock("complete", ChatBlockKind.TOOL, toolState = ToolRunState.SUCCESS),
            ChatBlock("message", ChatBlockKind.ASSISTANT, text = "done"),
        )

        val finalized = ChatTranscriptReducer.finalizeActiveTools(blocks, ToolRunState.CANCELLED)

        assertEquals(ToolRunState.CANCELLED, finalized[0].toolState)
        assertEquals(ToolRunState.CANCELLED, finalized[1].toolState)
        assertEquals(ToolRunState.SUCCESS, finalized[2].toolState)
        assertEquals(blocks[3], finalized[3])
    }
}
