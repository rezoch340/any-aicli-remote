import AnyAICLIRemoteCore
import SwiftUI

struct DeviceListView: View {
    @EnvironmentObject private var store: ChatStore
    let onSelectDevice: (SavedDevice) -> Void
    @State private var editedDevice: SavedDevice?
    @State private var showsDeviceEditor = false
    @State private var devicePendingDeletion: SavedDevice?
    @State private var showsQRCodeScanner = false

    var body: some View {
        List {
            if !store.deviceMessage.isEmpty {
                Section {
                    Label(store.deviceMessage, systemImage: messageIcon)
                        .font(.footnote)
                        .foregroundStyle(messageColor)
                }
            }

            Section("已配对设备") {
                ForEach(store.devices) { device in
                    Button {
                        onSelectDevice(device)
                    } label: {
                        DeviceRow(
                            device: device,
                            status: status(for: device.id)
                        )
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("device-row-\(device.name)")
                    .disabled(store.connection == .connecting)
                    .swipeActions(edge: .trailing) {
                        Button("删除", role: .destructive) {
                            devicePendingDeletion = device
                        }
                        .disabled(store.connection == .connecting)
                        Button("编辑") {
                            editedDevice = device
                            showsDeviceEditor = true
                        }
                        .tint(.blue)
                        .disabled(store.connection == .connecting)
                    }
                }
            }
        }
        .overlay {
            if store.devices.isEmpty {
                ContentUnavailableView {
                    Label("还没有设备", systemImage: "macbook.and.iphone")
                } description: {
                    Text("扫描启动器二维码，或手动添加 \(ProductIdentifiers.displayName) 设备。")
                } actions: {
                    Button("扫描二维码") { showsQRCodeScanner = true }
                        .buttonStyle(.borderedProminent)
                        .tint(.cyan)
                        .disabled(store.connection == .connecting)
                    Button("手动添加设备") {
                        editedDevice = nil
                        showsDeviceEditor = true
                    }
                    .buttonStyle(.bordered)
                    .disabled(store.connection == .connecting)
                }
            }
        }
        .navigationTitle("设备")
        .toolbar {
            ToolbarItemGroup(placement: .topBarTrailing) {
                Button {
                    Task { await store.refreshDeviceHealth() }
                } label: {
                    Label("刷新状态", systemImage: "arrow.clockwise")
                }
                Button {
                    editedDevice = nil
                    showsDeviceEditor = true
                } label: {
                    Label("添加设备", systemImage: "plus")
                }
                .disabled(store.connection == .connecting)
                Button {
                    showsQRCodeScanner = true
                } label: {
                    Label("扫描二维码", systemImage: "qrcode.viewfinder")
                }
                .disabled(store.connection == .connecting)
            }
        }
        .task(id: store.devices) {
            while !Task.isCancelled {
                await store.refreshDeviceHealth()
                do {
                    try await Task.sleep(nanoseconds: UInt64(store.healthPollingInterval * 1_000_000_000))
                } catch {
                    return
                }
            }
        }
        .sheet(isPresented: $showsDeviceEditor) {
            PairingView(device: editedDevice)
        }
        .sheet(isPresented: $showsQRCodeScanner) {
            QRCodeScannerView()
        }
        .confirmationDialog(
            "删除 \(devicePendingDeletion?.name ?? "这台设备")？",
            isPresented: Binding(
                get: { devicePendingDeletion != nil },
                set: { if !$0 { devicePendingDeletion = nil } }
            ),
            titleVisibility: .visible
        ) {
            Button("删除设备", role: .destructive) {
                if let devicePendingDeletion {
                    do {
                        try store.deleteDevice(devicePendingDeletion.id)
                    } catch {
                        store.reportDeviceError(error)
                    }
                }
                devicePendingDeletion = nil
            }
            Button("取消", role: .cancel) { devicePendingDeletion = nil }
        } message: {
            Text("设备配置与钥匙串中的配对 Key 都会被删除。")
        }
    }

    private var messageIcon: String {
        store.deviceMessageIsError ? "exclamationmark.triangle.fill" : "checkmark.circle.fill"
    }

    private var messageColor: Color {
        store.deviceMessageIsError ? .orange : .secondary
    }

    private func status(for deviceID: UUID) -> DeviceRow.Status {
        if store.activeDeviceID == deviceID, store.connection == .connecting {
            return .connecting
        }
        switch store.deviceHealthStatus(for: deviceID) {
        case .checking: return .checking
        case .online: return .online
        case .offline: return .offline
        }
    }
}

private struct DeviceRow: View {
    enum Status: Equatable {
        case offline
        case checking
        case connecting
        case online
    }

    let device: SavedDevice
    let status: Status

    var body: some View {
        HStack(spacing: 14) {
            ZStack {
                RoundedRectangle(cornerRadius: 14)
                    .fill(statusColor.opacity(0.15))
                    .frame(width: 50, height: 50)
                Image(systemName: "desktopcomputer")
                    .font(.title3)
                    .foregroundStyle(statusColor)
            }
            VStack(alignment: .leading, spacing: 5) {
                Text(device.name)
                    .font(.headline)
                    .lineLimit(1)
                Text(device.baseURL.absoluteString)
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            Spacer(minLength: 8)
            switch status {
            case .checking, .connecting:
                HStack(spacing: 5) {
                    ProgressView().controlSize(.small).tint(.cyan)
                    Text(status == .connecting ? "连接中" : "检查中")
                        .font(.caption2.weight(.semibold))
                        .foregroundStyle(.cyan)
                }
            case .online, .offline:
                Text(status == .online ? "在线" : "离线")
                    .font(.caption2.weight(.semibold))
                    .foregroundStyle(statusColor)
                    .padding(.horizontal, 8)
                    .padding(.vertical, 5)
                    .background(statusColor.opacity(0.12), in: Capsule())
            }
            Image(systemName: "chevron.right")
                .font(.caption)
                .foregroundStyle(.tertiary)
        }
        .contentShape(Rectangle())
        .padding(.vertical, 5)
    }

    private var statusColor: Color {
        switch status {
        case .offline: return .secondary
        case .checking, .connecting: return .cyan
        case .online: return .green
        }
    }
}
