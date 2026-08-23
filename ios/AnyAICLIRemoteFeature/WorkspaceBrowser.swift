import AnyAICLIRemoteCore
import Foundation

@MainActor
extension ChatStore {
  func closeWorkspaceFilePicker(clearSelection: Bool = false) {
    filePickerVisible = false
    filePickerLoading = false
    filePickerError = nil
    filePickerPath = "."
    filePickerParent = nil
    filePickerDirectories = []
    filePickerFiles = []
    if clearSelection { selectedFiles = [] }
  }

  func browseWorkspace(path: String = ".") {
    guard let session = selectedSession,
      let context = currentSessionContext(sessionIdentity: session.id)
    else { return }
    filePickerVisible = true
    filePickerLoading = true
    filePickerError = nil
    Task { [weak self] in
      guard let self else { return }
      do {
        let query = [
          URLQueryItem(name: "providerId", value: session.providerID),
          URLQueryItem(name: "sessionId", value: session.sessionID),
          URLQueryItem(name: "path", value: path)
        ]
        let response = try await client.rest(path: "/api/fs/list", query: query)
        guard ownsSession(context) else { return }
        filePickerPath = response.string("path") ?? path
        filePickerParent = response.string("parent")
        filePickerDirectories = response.array("dirs").compactMap {
          WorkspaceFile(json: $0, directory: true)
        }
        filePickerFiles = response.array("files").compactMap {
          WorkspaceFile(json: $0, directory: false)
        }
        filePickerLoading = false
      } catch  where ownsSession(context) {
        filePickerLoading = false
        filePickerError = error.localizedDescription
      }
    }
  }

  func toggleFile(_ file: WorkspaceFile) {
    guard !file.directory else { return }
    selectedFiles =
      selectedFiles.contains(file) ? selectedFiles.filter { $0 != file } : selectedFiles + [file]
  }
  func removeFile(_ file: WorkspaceFile) { selectedFiles.removeAll { $0 == file } }

}
