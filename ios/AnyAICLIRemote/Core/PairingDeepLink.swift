import Foundation

struct PairingDeepLink {
    static let pairingHost = "pair"
    static let serviceURLField = "url"
    static let pairingKeyField = "key"
    static let displayNameField = "name"

    let serviceAddress: String
    let profile: ServerProfile
    let name: String?

    static func parse(_ link: URL) throws -> PairingDeepLink {
        guard LegacyCompatibility.supportsPairingScheme(link.scheme),
              link.host?.lowercased() == pairingHost,
              let components = URLComponents(url: link, resolvingAgainstBaseURL: false) else {
            throw ClientError.invalidAddress
        }
        let queryItems = components.queryItems ?? []
        guard let serviceAddress = queryItems.first(where: { $0.name == serviceURLField })?.value,
              !serviceAddress.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw ClientError.invalidAddress
        }
        let queryKey = queryItems.first(where: { $0.name == pairingKeyField })?.value ?? ""
        let profile = try ServerProfile.parse(address: serviceAddress, fallbackKey: queryKey)
        let name = queryItems.first(where: { $0.name == displayNameField })?.value?
            .trimmingCharacters(in: .whitespacesAndNewlines)
        return PairingDeepLink(serviceAddress: serviceAddress, profile: profile,
                               name: name?.isEmpty == false ? name : nil)
    }
}
