import AnyAICLIRemoteCore
import SwiftUI
import SwiftStreamingMarkdown

struct ChatBlockView: View {
    let block: ChatBlock
    let isStreaming: Bool
    let onRender: () -> Void
    let onPermissionAnswer: (String, String?) -> Void

    var body: some View {
        switch block.kind {
        case .user: UserBubble(block: block)
        case .assistant: AssistantMessage(block: block, isStreaming: isStreaming, onRender: onRender)
        case .thinking: ThinkingBlock(block: block)
        case .tool: ToolCapsule(block: block)
        case .permission: PermissionCard(block: block, onPermissionAnswer: onPermissionAnswer)
        case .plan: PlanBlock(block: block)
        case .system: SystemBlock(block: block)
        }
    }
}

private struct UserBubble: View {
    let block: ChatBlock

    var body: some View {
        HStack {
            Spacer(minLength: 60)
            VStack(alignment: .leading, spacing: 6) {
                if !block.text.isEmpty { Text(block.text) }
                ForEach(block.attachments) { file in
                    Label(file.name, systemImage: "paperclip").font(.caption)
                }
            }
                .font(.system(size: 16.5))
                .textSelection(.enabled)
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                .background(Color.secondary.opacity(0.16), in: RoundedRectangle(cornerRadius: 18))
                .contextMenu {
                    Button("复制", systemImage: "doc.on.doc") { UIPasteboard.general.string = block.text }
                }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 5)
    }
}

private struct AssistantMessage: View {
    let block: ChatBlock
    let isStreaming: Bool
    let onRender: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 9) {
            Label("助手", systemImage: "sparkles")
                .font(.subheadline.bold())
                .foregroundStyle(.cyan)
            MarkdownText(block.text, isStreaming: isStreaming, onRender: onRender)
        }
        .accessibilityIdentifier("assistant-message")
        .accessibilityLabel("助手")
        .accessibilityValue(block.text)
        .accessibilityElement(children: .contain)
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 16)
        .padding(.vertical, 8)
        .contextMenu {
            Button("复制", systemImage: "doc.on.doc") { UIPasteboard.general.string = block.text }
        }
    }
}

private struct ThinkingBlock: View {
    let block: ChatBlock
    @State private var expanded = false

    var body: some View {
        DisclosureGroup(isExpanded: $expanded) {
            Text(block.text)
                .font(.callout)
                .foregroundStyle(.secondary)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.top, 8)
        } label: {
            Label("思考过程", systemImage: "brain.head.profile")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
        }
        .padding(12)
        .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 12))
        .padding(.horizontal, 16)
        .padding(.vertical, 4)
    }
}

struct ToolCapsule: View {
    let block: ChatBlock
    @State private var showDetail = false

    var body: some View {
        Button { showDetail = true } label: {
            HStack(spacing: 9) {
                Image(systemName: Self.icon(for: block.title))
                    .foregroundStyle(color)
                Text(block.title)
                    .font(.system(size: 13, weight: .medium))
                    .lineLimit(1)
                Spacer()
                statusIcon
            }
            .padding(.horizontal, 12)
            .frame(height: 38)
            .background(Color.secondary.opacity(0.09), in: Capsule())
            .overlay(Capsule().stroke(Color.secondary.opacity(0.18), lineWidth: 0.5))
        }
        .buttonStyle(.plain)
        .padding(.horizontal, 16)
        .padding(.vertical, 3)
        .sheet(isPresented: $showDetail) {
            NavigationStack {
                ScrollView {
                    Text(block.detail.isEmpty ? "暂无工具输出" : block.detail)
                        .font(.system(.callout, design: .monospaced))
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding()
                }
                .navigationTitle(block.title)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar { Button("完成") { showDetail = false } }
            }
            .presentationDetents([.medium, .large])
        }
    }

    private var color: Color {
        switch block.toolState {
        case .failed: return .red
        case .cancelled: return .orange
        case .success: return .green
        default: return .cyan
        }
    }

    @ViewBuilder private var statusIcon: some View {
        switch block.toolState {
        case .pending, .running: ProgressView().controlSize(.small).tint(color)
        case .success: Image(systemName: "checkmark.circle.fill").foregroundStyle(color)
        case .failed: Image(systemName: "xmark.circle.fill").foregroundStyle(color)
        case .cancelled: Image(systemName: "minus.circle.fill").foregroundStyle(color)
        }
    }

    static func icon(for title: String) -> String {
        let value = title.lowercased()
        if value.contains("terminal") || value.contains("shell") || value.contains("command") { return "terminal" }
        if value.contains("browser") || value.contains("web") { return "globe" }
        if value.contains("write") { return "doc.badge.plus" }
        if value.contains("edit") { return "square.and.pencil" }
        if value.contains("read") || value.contains("file") { return "doc.text" }
        if value.contains("image") { return "photo" }
        return "wrench.and.screwdriver"
    }
}

private struct PermissionCard: View {
    let block: ChatBlock
    let onPermissionAnswer: (String, String?) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Label("需要确认", systemImage: "hand.raised.fill")
                .font(.headline)
                .foregroundStyle(.orange)
            Text(block.text).font(.callout)
            HStack {
                ForEach(block.options) { option in
                    Button(option.label) { onPermissionAnswer(block.id, option.id) }
                        .buttonStyle(.borderedProminent)
                        .tint(option.id.lowercased().contains("reject") ? .red : .cyan)
                }
                Button("取消", role: .cancel) { onPermissionAnswer(block.id, nil) }
                    .buttonStyle(.bordered)
            }
        }
        .padding(14)
        .background(Color.orange.opacity(0.09), in: RoundedRectangle(cornerRadius: 14))
        .overlay(RoundedRectangle(cornerRadius: 14).stroke(Color.orange.opacity(0.25)))
        .padding(.horizontal, 16)
        .padding(.vertical, 5)
    }
}

private struct PlanBlock: View {
    let block: ChatBlock
    var body: some View {
        Label {
            Text(block.text).font(.callout).textSelection(.enabled)
        } icon: {
            Image(systemName: "list.bullet.clipboard")
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .background(Color.indigo.opacity(0.09), in: RoundedRectangle(cornerRadius: 12))
        .padding(.horizontal, 16)
    }
}

private struct SystemBlock: View {
    let block: ChatBlock
    var body: some View {
        Text(block.text)
            .font(.caption)
            .foregroundStyle(.secondary)
            .frame(maxWidth: .infinity)
            .padding(.vertical, 5)
            .padding(.horizontal, 16)
    }
}

private struct MarkdownText: View {
    let text: String
    let isStreaming: Bool
    let onRender: () -> Void
    @StateObject private var streamingSource: LiveMarkdownSource
    @StateObject private var renderListener: RenderCompleteListener

    init(_ text: String, isStreaming: Bool, onRender: @escaping () -> Void = {}) {
        self.text = text
        self.isStreaming = isStreaming
        self.onRender = onRender
        _streamingSource = StateObject(wrappedValue: LiveMarkdownSource(initialText: text))
        _renderListener = StateObject(wrappedValue: RenderCompleteListener(onRender: onRender))
    }

    var body: some View {
        Group {
            if isStreaming {
                StreamedMarkdownView(source: streamingSource, listener: isStreaming ? renderListener : nil)
            } else {
                MarkdownView(text: text, listener: renderListener)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .onAppear {
            streamingSource.update(text)
        }
        .onChange(of: text) { _, updatedText in
            streamingSource.update(updatedText)
        }
    }
}

@MainActor
private final class RenderCompleteListener: NSObject, ObservableObject, MarkdownListener {
    private let onRenderComplete: () -> Void
    private var renderPending = false

    init(onRender: @escaping () -> Void) {
        onRenderComplete = onRender
    }

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
        text = AsyncStream(bufferingPolicy: .bufferingNewest(1)) { streamContinuation in
            capturedContinuation = streamContinuation
        }
        guard let capturedContinuation else {
            preconditionFailure("Unable to create Markdown stream")
        }
        continuation = capturedContinuation
        lastText = initialText
        continuation.yield(initialText)
    }

    func update(_ updatedText: String) {
        guard updatedText != lastText else { return }
        lastText = updatedText
        continuation.yield(updatedText)
    }

    deinit {
        continuation.finish()
    }
}
