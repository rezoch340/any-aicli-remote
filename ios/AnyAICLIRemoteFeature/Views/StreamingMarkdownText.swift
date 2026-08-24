import SwiftUI
import SwiftStreamingMarkdown

internal enum StreamingMarkdownRenderLifecycle: Equatable {
    case completed
    case streaming

    init(isStreaming: Bool) {
        self = isStreaming ? .streaming : .completed
    }

    var usesStreamingRenderer: Bool { self == .streaming }

    mutating func observe(isStreaming: Bool) {
        if isStreaming { self = .streaming }
    }
}

internal enum StreamingMarkdownRenderConfiguration {
    static let animatesStreamedText = true
    static let completed = MarkdownRenderConfig.default
    static let streaming = MarkdownRenderConfig.default.withShouldAnimateText(value: animatesStreamedText)
}

struct MarkdownText: View {
    private let text: String
    private let isStreaming: Bool
    private let onRender: () -> Void
    @State private var lifecycle: StreamingMarkdownRenderLifecycle
    @StateObject private var streamingSource: LiveMarkdownSource
    @StateObject private var renderListener: RenderCompleteListener

    init(_ text: String, isStreaming: Bool, onRender: @escaping () -> Void = {}) {
        self.text = text
        self.isStreaming = isStreaming
        self.onRender = onRender
        _lifecycle = State(initialValue: StreamingMarkdownRenderLifecycle(isStreaming: isStreaming))
        _streamingSource = StateObject(wrappedValue: LiveMarkdownSource(initialText: text))
        _renderListener = StateObject(wrappedValue: RenderCompleteListener(onRender: onRender))
    }

    var body: some View {
        Group {
            if lifecycle.usesStreamingRenderer {
                StreamedMarkdownView(source: streamingSource, config: StreamingMarkdownRenderConfiguration.streaming, listener: renderListener)
            } else {
                MarkdownView(text: text, config: StreamingMarkdownRenderConfiguration.completed, listener: renderListener)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .onAppear { streamingSource.update(text) }
        .onChange(of: text) { _, updatedText in streamingSource.update(updatedText) }
        .onChange(of: isStreaming) { _, updatedValue in lifecycle.observe(isStreaming: updatedValue) }
    }
}

@MainActor
private final class RenderCompleteListener: NSObject, ObservableObject, MarkdownListener {
    private let onRenderComplete: () -> Void
    private var renderPending = false

    init(onRender: @escaping () -> Void) { onRenderComplete = onRender }

    func onRender(markdown: RenderableDocument) async {
        guard !renderPending else { return }
        renderPending = true
        DispatchQueue.main.async {
            self.renderPending = false
            self.onRenderComplete()
        }
    }

    func onTableCopyTap(content: String) async {}
    func onTableDownloadTap(content: String) async {}
    func onContextMenuAppear(id: String, selectedContent: String) async {}
    func onContextMenuTap(id: String, selectedContent: String) async {}
    func onImageTap(image: MarkdownImage) async {}
}

private final class LiveMarkdownSource: ObservableObject, StreamedMarkdownSource {
    let text: AsyncStream<String>
    private let continuation: AsyncStream<String>.Continuation
    private var lastText: String

    init(initialText: String) {
        var capturedContinuation: AsyncStream<String>.Continuation?
        text = AsyncStream(bufferingPolicy: .bufferingNewest(1)) { capturedContinuation = $0 }
        guard let capturedContinuation else { preconditionFailure("Unable to create Markdown stream") }
        continuation = capturedContinuation
        lastText = initialText
        continuation.yield(initialText)
    }

    func update(_ updatedText: String) {
        guard updatedText != lastText else { return }
        lastText = updatedText
        continuation.yield(updatedText)
    }

    deinit { continuation.finish() }
}
