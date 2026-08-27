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
    if link.scheme?.lowercased() == ProductIdentifiers.pairingScheme,
      link.host?.lowercased() == pairingHost {
      return try parseProductDeepLink(link)
    }
    return try parseHTTPPairingURL(link)
  }

  private static func parseProductDeepLink(_ link: URL) throws -> PairingDeepLink {
    guard let components = URLComponents(url: link, resolvingAgainstBaseURL: false) else {
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
    return PairingDeepLink(
      serviceAddress: serviceAddress,
      profile: profile,
      name: normalizedName(in: queryItems))
  }

  private static func parseHTTPPairingURL(_ link: URL) throws -> PairingDeepLink {
    guard let components = URLComponents(url: link, resolvingAgainstBaseURL: false),
      let scheme = components.scheme?.lowercased(),
      ["http", "https"].contains(scheme),
      components.host != nil
    else {
      throw ClientError.invalidAddress
    }
    let queryItems = components.queryItems ?? []
    let autoPair = queryItems.first(where: { $0.name == "auto" })?.value
    let queryKey = queryItems.first(where: { $0.name == pairingKeyField })?.value ?? ""
    guard autoPair == "1", !queryKey.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
      throw ClientError.invalidAddress
    }
    let serviceAddress = link.absoluteString
    let profile = try ServerProfile.parse(address: serviceAddress, fallbackKey: queryKey)
    return PairingDeepLink(
      serviceAddress: profile.baseURL.absoluteString,
      profile: profile,
      name: normalizedName(in: queryItems))
  }

  private static func normalizedName(in queryItems: [URLQueryItem]) -> String? {
    let name = queryItems.first(where: { $0.name == displayNameField })?.value?
      .trimmingCharacters(in: .whitespacesAndNewlines)
    return name?.isEmpty == false ? name : nil
  }
}
