import AnyAICLIRemoteCore
import SwiftUI
import Foundation

struct ChatView: View {
    @EnvironmentObject private var store: ChatStore
    let session: SessionSummary
    @State private var draft = ""
    @State private var shouldFollow = true
    @State private var scrollRequestRevision = 0
    @State private var hasRequestedOpen = false

    private var isPreparingSession: Bool {
        !hasRequestedOpen || store.isSessionLoading || store.selectedSession?.id != session.id
    }

    private var activeTool: ChatBlock? {
        store.blocks.last(where: { $0.kind == .tool && [.pending, .running].contains($0.toolState) })
    }

    private var streamingAssistantID: String? {
        guard store.isBusy else { return nil }
        return store.blocks.last(where: { $0.kind == .assistant })?.id
    }

    private var chatContent: some View {
        ChatMessageCollectionView(
            sessionIdentity: session.id,
            blocks: store.blocks,
            streamingAssistantID: streamingAssistantID,
            isBusy: store.isBusy,
            isFollowing: $shouldFollow,
            scrollRequestRevision: scrollRequestRevision,
            onPermissionAnswer: { blockID, optionID in
                store.answerPermission(blockID: blockID, optionID: optionID)
            }
        )
        .overlay(alignment: .bottomTrailing) {
            if !shouldFollow {
                Button {
                    shouldFollow = true
                    scrollRequestRevision += 1
                } label: {
                    Image(systemName: "arrow.down")
                        .frame(width: 40, height: 40)
                        .background(.ultraThinMaterial, in: Circle())
                }
                .padding(.trailing, 16)
                .padding(.bottom, 8)
            }
        }
        .safeAreaInset(edge: .bottom, spacing: 0) {
            VStack(spacing: 0) {
                if let activeTool { FloatingToolBar(block: activeTool, onStop: store.cancel) }
                ComposerView(
                    text: $draft,
                    isBusy: store.isBusy,
                    status: store.statusMessage,
                    attachments: store.selectedFiles,
                    onAttach: { store.browseWorkspace() },
                    onRemove: { store.removeFile($0) },
                    onSend: {
                        let outgoing = draft
                        draft = ""
                        shouldFollow = true
                        scrollRequestRevision += 1
                        store.send(outgoing)
                    },
                    onStop: { store.cancel() }
                )
            }
        }
    }

    var body: some View {
        Group {
            if isPreparingSession {
                ProgressView("同步历史")
                    .accessibilityIdentifier("session-history-loading")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                chatContent
            }
        }
        .navigationTitle(session.title)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItemGroup(placement: .topBarTrailing) {
                Text(store.modelState.currentModelID.isEmpty ? "自动" : store.modelState.currentModelID)
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                Menu {
                    ForEach(store.modelState.effortLevels, id: \.self) { effort in
                        Button {
                            store.setEffort(effort)
                        } label: {
                            if effort == store.modelState.effort {
                                Label(effort, systemImage: "checkmark")
                            } else {
                                Text(effort)
                            }
                        }
                    }
                } label: {
                    Text(store.modelState.effort.uppercased())
                        .font(.caption.bold())
                        .padding(.horizontal, 8)
                        .padding(.vertical, 4)
                        .background(.thinMaterial, in: Capsule())
                }
            }
        }
        .task {
            store.openSession(session)
            hasRequestedOpen = true
        }
        .onDisappear { store.closeSession(session.id) }
        .sheet(
            isPresented: $store.filePickerVisible,
            onDismiss: { store.closeWorkspaceFilePicker() },
            content: {
                WorkspaceFilePickerView().environmentObject(store)
            }
        )
    }
}

private struct ComposerView: View {
    @Binding var text: String
    let isBusy: Bool
    let status: String
    let attachments: [WorkspaceFile]
    let onAttach: () -> Void
    let onRemove: (WorkspaceFile) -> Void
    let onSend: () -> Void
    let onStop: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            if !attachments.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    LazyHStack(spacing: 8) {
                        ForEach(attachments) { file in
                            ComposerAttachmentCard(file: file) { onRemove(file) }
                        }
                    }
                    .padding(.vertical, 1)
                }
            }
            if !status.isEmpty {
                Text(status)
                    .accessibilityIdentifier("chat-status")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, 4)
            }
            HStack(alignment: .bottom, spacing: 10) {
                Button(action: onAttach) {
                    Image(systemName: "plus")
                        .frame(width: 34, height: 34)
                        .background(Color.secondary.opacity(0.14), in: Circle())
                }
                .disabled(isBusy)

                TextField("发送消息", text: $text, axis: .vertical)
                    .accessibilityIdentifier("chat-composer")
                    .lineLimit(1...7)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 9)
                    .background(Color.secondary.opacity(0.12), in: RoundedRectangle(cornerRadius: 18))
                    .onSubmit {
                        if (!text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || !attachments.isEmpty) && !isBusy {
                            onSend()
                        }
                    }

                Button(action: isBusy ? onStop : onSend) {
                    Image(systemName: isBusy ? "stop.fill" : "arrow.up")
                        .font(.system(size: 15, weight: .bold))
                        .foregroundStyle(isBusy ? Color.primary : Color.black)
                        .frame(width: 36, height: 36)
                        .background(isBusy ? Color.secondary.opacity(0.18) : Color.cyan, in: Circle())
                }
                .accessibilityLabel(isBusy ? "停止生成" : "发送")
                .disabled(!isBusy && text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && attachments.isEmpty)
            }
        }
        .padding(.horizontal, 12)
        .padding(.top, 8)
        .padding(.bottom, 8)
        .background(.bar)
    }
}

private struct ComposerAttachmentCard: View {
    let file: WorkspaceFile
    let onRemove: () -> Void

    private var fileSize: String {
        guard file.size > 0 else { return "" }
        return ByteCountFormatter.string(fromByteCount: file.size, countStyle: .file)
    }

    var body: some View {
        HStack(spacing: 7) {
            Image(systemName: "paperclip")
                .foregroundStyle(.cyan)
            VStack(alignment: .leading, spacing: 2) {
                Text(file.name)
                    .font(.caption.weight(.semibold))
                    .lineLimit(1)
                Text([file.relativePath, fileSize].filter { !$0.isEmpty }.joined(separator: " · "))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            Spacer(minLength: 0)
            Button(action: onRemove) {
                Image(systemName: "xmark.circle.fill")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("移除附件 \(file.name)")
        }
        .padding(.horizontal, 9)
        .padding(.vertical, 7)
        .frame(width: 220, alignment: .leading)
        .background(Color.secondary.opacity(0.1), in: RoundedRectangle(cornerRadius: 10))
        .overlay(RoundedRectangle(cornerRadius: 10).stroke(Color.secondary.opacity(0.18)))
    }
}

private struct FloatingToolBar: View {
    let block: ChatBlock
    let onStop: () -> Void

    var body: some View {
        HStack(spacing: 10) {
            ProgressView().controlSize(.small).tint(.cyan)
            Image(systemName: ToolCapsule.icon(for: block.title)).foregroundStyle(.cyan)
            Text(block.title).font(.caption.weight(.medium)).lineLimit(1)
            Spacer()
            Button(action: onStop) {
                Image(systemName: "stop.fill").font(.caption)
                    .frame(width: 30, height: 30)
                    .background(Color.red.opacity(0.15), in: Circle())
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 8)
        .background(.ultraThinMaterial)
        .overlay(alignment: .top) { Divider() }
    }
}
