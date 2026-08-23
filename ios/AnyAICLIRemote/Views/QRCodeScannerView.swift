import SwiftUI
import VisionKit

struct QRCodeScannerView: View {
    @EnvironmentObject private var store: ChatStore
    @Environment(\.dismiss) private var dismiss
    @State private var errorMessage = ""

    var body: some View {
        NavigationStack {
            Group {
                if DataScannerViewController.isSupported && DataScannerViewController.isAvailable {
                    QRCodeScannerRepresentable { payload in
                        guard let url = URL(string: payload) else {
                            errorMessage = "二维码内容不是有效的配对链接"
                            return false
                        }
                        let imported = store.importPairingDeepLink(url)
                        if imported { dismiss() }
                        return imported
                    } onError: { message in
                        errorMessage = message
                    }
                    .ignoresSafeArea()
                } else {
                    ContentUnavailableView("无法使用相机", systemImage: "camera.slash", description: Text("此设备或当前状态不支持二维码扫描，请使用手动添加。"))
                }
            }
            .navigationTitle("扫描二维码")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
            }
            .alert("扫描失败", isPresented: Binding(get: { !errorMessage.isEmpty }, set: { if !$0 { errorMessage = "" } })) {
                Button("确定", role: .cancel) { errorMessage = "" }
            } message: { Text(errorMessage) }
        }
    }
}

private struct QRCodeScannerRepresentable: UIViewControllerRepresentable {
    let onPayload: (String) -> Bool
    let onError: (String) -> Void

    func makeCoordinator() -> Coordinator { Coordinator(onPayload: onPayload, onError: onError) }

    func makeUIViewController(context: Context) -> DataScannerViewController {
        let scanner = DataScannerViewController(recognizedDataTypes: [.barcode(symbologies: [.qr])], qualityLevel: .balanced, recognizesMultipleItems: false, isHighFrameRateTrackingEnabled: false, isPinchToZoomEnabled: true, isGuidanceEnabled: true, isHighlightingEnabled: true)
        scanner.delegate = context.coordinator
        do { try scanner.startScanning() }
        catch { context.coordinator.onError("相机启动失败：\(error.localizedDescription)") }
        return scanner
    }

    func updateUIViewController(_ uiViewController: DataScannerViewController, context: Context) {}

    final class Coordinator: NSObject, DataScannerViewControllerDelegate {
        let onPayload: (String) -> Bool
        let onError: (String) -> Void
        private var completed = false
        init(onPayload: @escaping (String) -> Bool, onError: @escaping (String) -> Void) { self.onPayload = onPayload; self.onError = onError }
        func dataScanner(_ dataScanner: DataScannerViewController, didAdd addedItems: [RecognizedItem], allItems: [RecognizedItem]) {
            for item in addedItems {
                if case .barcode(let barcode) = item, let payload = barcode.payloadStringValue, !completed {
                    if onPayload(payload) {
                        completed = true
                        dataScanner.stopScanning()
                        return
                    }
                    onError("二维码无效，请扫描配对二维码")
                }
            }
        }
        func dataScanner(_ dataScanner: DataScannerViewController, becameUnavailableWithError error: DataScannerViewController.ScanningUnavailable) {
            onError("二维码扫描不可用：\(error.localizedDescription)")
        }
    }
}
