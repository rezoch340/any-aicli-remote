import CoreImage
import CoreImage.CIFilterBuiltins
import SwiftUI

struct QRCodeView: View {
  let payload: String

  var body: some View {
    Group {
      if let image = QRCodeRenderer.image(for: payload) {
        Image(nsImage: image)
          .resizable()
          .interpolation(.none)
          .antialiased(false)
      } else {
        Image(systemName: "qrcode")
          .font(.system(size: 64))
          .foregroundStyle(.secondary)
      }
    }
    .aspectRatio(1, contentMode: .fit)
    .accessibilityLabel("Any AI CLI Remote 配对二维码")
  }
}

private enum QRCodeRenderer {
  static func image(for payload: String) -> NSImage? {
    let filter = CIFilter.qrCodeGenerator()
    filter.message = Data(payload.utf8)
    filter.correctionLevel = "M"
    guard let output = filter.outputImage else { return nil }

    let quietZone: CGFloat = 4
    let scale = max(1, floor(768 / (output.extent.width + quietZone * 2)))
    let translated =
      output
      .transformed(by: CGAffineTransform(scaleX: scale, y: scale))
      .transformed(by: CGAffineTransform(translationX: quietZone * scale, y: quietZone * scale))
    let side = (output.extent.width + quietZone * 2) * scale
    let background = CIImage(color: .white).cropped(
      to: CGRect(x: 0, y: 0, width: side, height: side))
    let composed = translated.composited(over: background)
    guard
      let cgImage = CIContext(options: [.useSoftwareRenderer: false])
        .createCGImage(composed, from: composed.extent)
    else { return nil }
    return NSImage(cgImage: cgImage, size: NSSize(width: side, height: side))
  }
}
