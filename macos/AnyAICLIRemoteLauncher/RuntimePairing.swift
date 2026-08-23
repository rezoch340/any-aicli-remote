import Foundation

struct RuntimePairing: Equatable {
  let httpURL: URL
  let deepLinkURL: URL

  init(configuration: RuntimeConfiguration) throws {
    guard let httpValue = configuration.pairingURL,
      let deepLinkValue = configuration.pairingDeepLink,
      let parsedHTTPURL = URL(string: httpValue),
      let parsedDeepLinkURL = URL(string: deepLinkValue),
      let scheme = parsedHTTPURL.scheme?.lowercased(),
      ["http", "https"].contains(scheme),
      !(parsedHTTPURL.host ?? "").isEmpty,
      parsedDeepLinkURL.scheme == ProductIdentifier.deepLinkScheme,
      parsedDeepLinkURL.host == ProductIdentifier.deepLinkHost
    else {
      throw RuntimePairingError.invalidPayload
    }
    httpURL = parsedHTTPURL
    deepLinkURL = parsedDeepLinkURL
  }
}

enum RuntimePairingError: LocalizedError, Equatable {
  case invalidPayload

  var errorDescription: String? {
    "daemon 返回的配对地址无效"
  }
}
