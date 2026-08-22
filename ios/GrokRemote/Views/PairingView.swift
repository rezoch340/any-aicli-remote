import SwiftUI

struct PairingView: View {
    @EnvironmentObject private var store: ChatStore
    @FocusState private var focused: Bool

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 24) {
                    VStack(alignment: .leading, spacing: 8) {
                        Image(systemName: "bolt.horizontal.circle.fill")
                            .font(.system(size: 54))
                            .foregroundStyle(.cyan)
                        Text("连接 Grok Remote")
                            .font(.largeTitle.bold())
                        Text("粘贴 connect.url 的整条链接，或者分别输入服务地址与配对 Key。")
                            .foregroundStyle(.secondary)
                    }

                    VStack(spacing: 14) {
                        LabeledContent("服务地址") {
                            TextField("http://192.168.1.100:2421", text: $store.address)
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()
                                .multilineTextAlignment(.trailing)
                                .focused($focused)
                        }
                        Divider()
                        LabeledContent("配对 Key") {
                            SecureField("connect.url 中的 key", text: $store.pairingKey)
                                .textInputAutocapitalization(.never)
                                .multilineTextAlignment(.trailing)
                        }
                        Divider()
                        LabeledContent("默认目录") {
                            TextField("~", text: $store.defaultCwd)
                                .textInputAutocapitalization(.never)
                                .multilineTextAlignment(.trailing)
                        }
                    }
                    .padding()
                    .background(.thinMaterial, in: RoundedRectangle(cornerRadius: 20))

                    Button {
                        focused = false
                        Task { await store.connect() }
                    } label: {
                        HStack {
                            if store.connection == .connecting { ProgressView().tint(.black) }
                            Text(store.connection == .connecting ? "连接中" : "连接")
                                .fontWeight(.semibold)
                        }
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 10)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(.cyan)
                    .foregroundStyle(.black)
                    .disabled(store.connection == .connecting)

                    if case .failed(let message) = store.connection {
                        Label(message, systemImage: "exclamationmark.triangle.fill")
                            .font(.footnote)
                            .foregroundStyle(.orange)
                    }
                }
                .padding(24)
            }
            .background(Color(.systemGroupedBackground))
        }
    }
}
