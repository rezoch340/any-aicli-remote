import SwiftUI

struct ChatView: View {
    @EnvironmentObject private var store: ChatStore
    let session: SessionSummary
    @State private var draft = ""
    @State private var shouldFollow = true

    private var activeTool: ChatBlock? {
        store.blocks.last(where: { $0.kind == .tool && [.pending, .running].contains($0.toolState) })
    }

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(spacing: 4) {
                    ForEach(store.blocks) { block in
                        ChatBlockView(block: block)
                            .id(block.id)
                    }
                    Color.clear.frame(height: 1).id("chat-bottom")
                }
                .padding(.vertical, 12)
            }
            .scrollDismissesKeyboard(.interactively)
            .simultaneousGesture(DragGesture().onChanged { value in
                if value.translation.height > 8 { shouldFollow = false }
            })
            .onChange(of: store.blocks) { _, _ in
                guard shouldFollow else { return }
                withAnimation(.easeOut(duration: 0.2)) { proxy.scrollTo("chat-bottom", anchor: .bottom) }
            }
            .overlay(alignment: .bottomTrailing) {
                if !shouldFollow {
                    Button {
                        shouldFollow = true
                        withAnimation { proxy.scrollTo("chat-bottom", anchor: .bottom) }
                    } label: {
                        Image(systemName: "arrow.down")
                            .frame(width: 40, height: 40)
                            .background(.ultraThinMaterial, in: Circle())
                    }
                    .padding(.trailing, 16)
                    .padding(.bottom, 8)
                }
            }
        }
        .navigationTitle(session.title)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItemGroup(placement: .topBarTrailing) {
                Text(store.modelState.currentModelID)
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
        .safeAreaInset(edge: .bottom, spacing: 0) {
            VStack(spacing: 0) {
                if let activeTool { FloatingToolBar(block: activeTool, onStop: store.cancel) }
                ComposerView(text: $draft, isBusy: store.isBusy, status: store.statusMessage) {
                    let outgoing = draft
                    draft = ""
                    shouldFollow = true
                    store.send(outgoing)
                } onStop: {
                    store.cancel()
                }
            }
        }
        .task { await store.openSession(session) }
    }
}

private struct ComposerView: View {
    @Binding var text: String
    let isBusy: Bool
    let status: String
    let onSend: () -> Void
    let onStop: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            if !status.isEmpty {
                Text(status)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, 4)
            }
            HStack(alignment: .bottom, spacing: 10) {
                Button {} label: {
                    Image(systemName: "plus")
                        .frame(width: 34, height: 34)
                        .background(Color.secondary.opacity(0.14), in: Circle())
                }
                .disabled(true)

                TextField("给 Grok 发送消息", text: $text, axis: .vertical)
                    .lineLimit(1...7)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 9)
                    .background(Color.secondary.opacity(0.12), in: RoundedRectangle(cornerRadius: 18))
                    .onSubmit { if !text.isEmpty && !isBusy { onSend() } }

                Button(action: isBusy ? onStop : onSend) {
                    Image(systemName: isBusy ? "stop.fill" : "arrow.up")
                        .font(.system(size: 15, weight: .bold))
                        .foregroundStyle(isBusy ? Color.primary : Color.black)
                        .frame(width: 36, height: 36)
                        .background(isBusy ? Color.secondary.opacity(0.18) : Color.cyan, in: Circle())
                }
                .disabled(!isBusy && text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
        }
        .padding(.horizontal, 12)
        .padding(.top, 8)
        .padding(.bottom, 8)
        .background(.bar)
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
