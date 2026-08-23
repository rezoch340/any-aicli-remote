import SwiftUI

struct PairingView: View {
    @EnvironmentObject private var store: ChatStore
    @Environment(\.dismiss) private var dismiss
    let device: SavedDevice?
    @State private var name: String
    @State private var address: String
    @State private var pairingKey = ""
    @State private var errorMessage = ""

    init(device: SavedDevice?) {
        self.device = device
        _name = State(initialValue: device?.name ?? "")
        _address = State(initialValue: device?.baseURL.absoluteString ?? "")
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("名称（例如：工作室 Mac）", text: $name)
                    TextField("http://mac.local:2421", text: $address)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                    SecureField("配对 Key", text: $pairingKey)
                        .textContentType(.oneTimeCode)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                } header: {
                    Text("设备")
                } footer: {
                    Text("配对 Key 仅保存在系统钥匙串中。服务地址支持 HTTP、HTTPS 和自定义端口。")
                }

                if !errorMessage.isEmpty {
                    Section {
                        Label(errorMessage, systemImage: "exclamationmark.triangle.fill")
                            .foregroundStyle(.orange)
                    }
                }
            }
            .navigationTitle(device == nil ? "添加设备" : "编辑设备")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("保存") { save() }
                        .disabled(address.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
            }
            .onAppear {
                if let device, pairingKey.isEmpty {
                    do {
                        pairingKey = try store.pairingKey(for: device.id)
                    } catch {
                        errorMessage = error.localizedDescription
                    }
                }
            }
        }
    }

    private func save() {
        do {
            _ = try store.saveDevice(
                id: device?.id,
                name: name,
                address: address,
                pairingKey: pairingKey
            )
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
