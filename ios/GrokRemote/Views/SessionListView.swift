import SwiftUI

struct SessionListView: View {
    @EnvironmentObject private var store: ChatStore
    @Binding var path: [String]
    @State private var showNewSession = false
    @State private var cwd = ""

    var body: some View {
        List {
            if store.connection == .reconnecting {
                Label("连接中断，正在后台重连", systemImage: "wifi.exclamationmark")
                    .foregroundStyle(.orange)
            }
            ForEach(store.sessions) { session in
                Button {
                    path.append(session.id)
                } label: {
                    SessionRow(session: session)
                }
                .buttonStyle(.plain)
            }
        }
        .overlay {
            if store.sessions.isEmpty {
                ContentUnavailableView("还没有会话", systemImage: "bubble.left.and.bubble.right", description: Text("创建一个会话开始连接 Grok"))
            }
        }
        .navigationTitle("Grok Remote")
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
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
                    cwd = store.defaultCwd
                    showNewSession = true
                } label: {
                    Image(systemName: "square.and.pencil")
                }
            }
        }
        .refreshable { try? await store.refreshSessions() }
        .sheet(isPresented: $showNewSession) {
            NavigationStack {
                Form {
                    Section("工作目录") {
                        TextField("~/project", text: $cwd)
                            .textInputAutocapitalization(.never)
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
                                if let session = await store.createSession(cwd: cwd) {
                                    showNewSession = false
                                    path.append(session.id)
                                }
                            }
                        }
                        .disabled(cwd.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
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
                    Text(session.cwd.isEmpty ? session.id.prefix(8) + "…" : session.cwd)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            Spacer()
            if let date = session.updatedAt {
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
