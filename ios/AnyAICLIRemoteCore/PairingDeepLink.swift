import Foundation

public struct PairingDeepLink {
  public static let pairingHost = "pair"
  public static let serviceURLField = "url"
  public static let pairingKeyField = "key"
  public static let displayNameField = "name"

  public let serviceAddress: String
  public let profile: ServerProfile
  public let name: String?

  public static func parse(_ link: URL) throws -> PairingDeepLink {
    guard link.scheme?.lowercased() == ProductIdentifiers.pairingScheme,
      link.host?.lowercased() == pairingHost,
      let components = URLComponents(url: link, resolvingAgainstBaseURL: false)
    else {
      throw ClientError.invalidAddress
    }
    let queryItems = components.queryItems ?? []
    guard let serviceAddress = queryItems.first(where: { $0.name == serviceURLField })?.value,
      !serviceAddress.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    else {
      throw ClientError.invalidAddress
    }
    let queryKey = queryItems.first(where: { $0.name == pairingKeyField })?.value ?? ""
    let profile = try ServerProfile.parse(address: serviceAddress, fallbackKey: queryKey)
    let name = queryItems.first(where: { $0.name == displayNameField })?.value?
      .trimmingCharacters(in: .whitespacesAndNewlines)
    return PairingDeepLink(
      serviceAddress: serviceAddress, profile: profile,
      name: name?.isEmpty == false ? name : nil)
  }
}
