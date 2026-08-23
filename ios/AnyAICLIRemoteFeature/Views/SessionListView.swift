import AnyAICLIRemoteCore
import SwiftUI

struct SessionListView: View {
    @EnvironmentObject private var store: ChatStore
    let onOpenSession: (SessionSummary) -> Void
    @State private var showNewSession = false
    @State private var workingDirectory = "~"

    var body: some View {
        List {
            if !store.statusMessage.isEmpty, store.statusMessage != "已连接" {
                Text(store.statusMessage)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            ForEach(store.sessions) { session in
                Button {
                    onOpenSession(session)
                } label: {
                    SessionRow(session: session)
                }
                .buttonStyle(.plain)
            }
        }
        .overlay {
            if store.sessions.isEmpty {
                ContentUnavailableView(
                    "还没有会话",
                    systemImage: "bubble.left.and.bubble.right",
                    description: Text("创建一个 \(ProductIdentifiers.displayName) 会话")
                )
            }
        }
        .navigationTitle(store.activeDevice?.name ?? "会话")
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button("断开连接", role: .destructive) { store.disconnect() }
                } label: {
                    Label(store.connection.label, systemImage: store.connection == .connected ? "circle.fill" : "arrow.triangle.2.circlepath")
                        .foregroundStyle(store.connection == .connected ? .green : .orange)
                }
            }
            ToolbarItemGroup(placement: .topBarTrailing) {
                Button { Task { try? await store.refreshSessions() } } label: {
                    Image(systemName: "arrow.clockwise")
                }
                Button {
                    workingDirectory = "~"
                    showNewSession = true
                } label: {
                    Image(systemName: "square.and.pencil")
                }
                .accessibilityLabel("新建会话")
            }
        }
        .refreshable { try? await store.refreshSessions() }
        .sheet(isPresented: $showNewSession) {
            NavigationStack {
                Form {
                    Section("工作目录") {
                        TextField("~ 或 ~/project", text: $workingDirectory)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                    }
                }
                .navigationTitle("新建会话")
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("取消") { showNewSession = false }
                    }
                    ToolbarItem(placement: .confirmationAction) {
                        Button("创建") {
                            Task {
                                if await store.createSession(workingDirectory: workingDirectory) {
                                    showNewSession = false
                                    if let session = store.selectedSession {
                                        onOpenSession(session)
                                    }
                                }
                            }
                        }
                        .disabled(workingDirectory.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                    }
                }
            }
            .presentationDetents([.medium])
        }
    }
}

private struct SessionRow: View {
    let session: SessionSummary

    var body: some View {
        HStack(spacing: 12) {
            ZStack {
                RoundedRectangle(cornerRadius: 12)
                    .fill(session.isResident ? Color.green.opacity(0.16) : Color.secondary.opacity(0.12))
                    .frame(width: 44, height: 44)
                Image(systemName: session.isResident ? "bolt.fill" : "bubble.left.fill")
                    .foregroundStyle(session.isResident ? .green : .secondary)
            }
            VStack(alignment: .leading, spacing: 4) {
                Text(session.title).font(.headline).lineLimit(1)
                HStack(spacing: 5) {
                    if session.isResident { Text("LIVE").font(.caption2.bold()).foregroundStyle(.green) }
                    Text(
                        session.projectDirectory.isEmpty
                            ? "\(session.sessionID.prefix(8))…"
                            : session.projectDirectory
                    )
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            Spacer()
            if let date = session.lastActiveAt ?? session.createdAt {
                Text(date, style: .relative)
                    .font(.caption2)
                    .foregroundStyle(.tertiary)
            }
            Image(systemName: "chevron.right").font(.caption).foregroundStyle(.tertiary)
        }
        .contentShape(Rectangle())
        .padding(.vertical, 4)
    }
}
