import SwiftUI

struct WorkspaceFilePickerView: View {
    @EnvironmentObject private var store: ChatStore

    var body: some View {
        NavigationStack {
            List {
                Section("当前路径") {
                    ScrollView(.horizontal, showsIndicators: false) {
                        Text(store.filePickerPath)
                            .font(.caption.monospaced())
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                            .fixedSize(horizontal: true, vertical: false)
                    }
                    .accessibilityLabel("当前路径 \(store.filePickerPath)")
                }

                if store.filePickerLoading {
                    Section { ProgressView("正在读取工作区") }
                }

                if let error = store.filePickerError {
                    Section("读取失败") {
                        Label(error, systemImage: "exclamationmark.triangle")
                            .foregroundStyle(.red)
                    }
                }

                if let parent = store.filePickerParent {
                    Section {
                        Button { store.browseWorkspace(path: parent) } label: {
                            Label("返回上级", systemImage: "arrow.turn.up.left")
                        }
                    }
                }

                if !store.filePickerDirectories.isEmpty {
                    Section("目录") {
                        ForEach(store.filePickerDirectories) { file in
                            Button { store.browseWorkspace(path: file.path) } label: {
                                Label(file.name, systemImage: "folder")
                            }
                            .accessibilityLabel("打开目录 \(file.name)")
                        }
                    }
                }

                if !store.filePickerFiles.isEmpty {
                    Section("文件") {
                        ForEach(store.filePickerFiles) { file in
                            let selected = store.selectedFiles.contains(file)
                            Button { store.toggleFile(file) } label: {
                                HStack(spacing: 10) {
                                    Image(systemName: "doc")
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text(file.name).lineLimit(1)
                                        Text(file.relativePath)
                                            .font(.caption2)
                                            .foregroundStyle(.secondary)
                                            .lineLimit(1)
                                    }
                                    Spacer(minLength: 8)
                                    if selected { Image(systemName: "checkmark.circle.fill").foregroundStyle(.tint) }
                                }
                            }
                            .accessibilityLabel(selected ? "已选择 \(file.name)" : "选择 \(file.name)")
                        }
                    }
                }

                if !store.filePickerLoading,
                   store.filePickerError == nil,
                   store.filePickerDirectories.isEmpty,
                   store.filePickerFiles.isEmpty {
                    Section { Label("此目录没有可选文件", systemImage: "folder.badge.questionmark") }
                }
            }
            .navigationTitle("选择工作区文件")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("取消") { store.closeWorkspaceFilePicker(clearSelection: true) }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("完成") { store.closeWorkspaceFilePicker() }
                }
            }
        }
    }
}
