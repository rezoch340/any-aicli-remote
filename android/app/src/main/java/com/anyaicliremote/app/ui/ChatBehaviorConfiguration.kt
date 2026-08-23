package com.anyaicliremote.app.ui

import kotlin.time.Duration
import kotlin.time.Duration.Companion.milliseconds

data class ChatUiBehaviorConfiguration(
    val streamScrollSampleInterval: Duration,
    val userMessageScrollDelay: Duration,
    val streamCompletionScrollDelay: Duration,
    val streamingTextUpdateDelay: Duration,
)

val DefaultChatUiBehaviorConfiguration = ChatUiBehaviorConfiguration(
    streamScrollSampleInterval = 120.milliseconds,
    userMessageScrollDelay = 16.milliseconds,
    streamCompletionScrollDelay = 220.milliseconds,
    streamingTextUpdateDelay = 80.milliseconds,
)
