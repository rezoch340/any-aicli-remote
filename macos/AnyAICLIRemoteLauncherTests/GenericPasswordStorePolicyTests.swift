import Foundation
import Security
import XCTest

final class GenericPasswordStorePolicyTests: XCTestCase {
    func testPreferredPolicyReadsExistingFileBasedItem() throws {
        let location = makeLocation()
        defer { deleteAllRepresentations(at: location) }

        let expectedData = Data("file-based".utf8)
        try GenericPasswordStore.save(expectedData, at: location, policy: .fileBased)

        XCTAssertEqual(
            try GenericPasswordStore.read(at: location, policy: .dataProtectionPreferred),
            expectedData
        )
    }

    func testPreferredPolicyFallsBackWithoutDataProtectionEntitlement() throws {
        let location = makeLocation()
        defer { deleteAllRepresentations(at: location) }

        do {
            try GenericPasswordStore.save(
                Data("probe".utf8),
                at: location,
                policy: .dataProtectionRequired
            )
            throw XCTSkip("Test host has a profile-authorized data-protection keychain entitlement")
        } catch let storeError as GenericPasswordStoreError where storeError.isMissingEntitlement {
            // This is the expected environment for the local ad-hoc build.
        }

        let expectedData = Data("fallback".utf8)
        try GenericPasswordStore.save(
            expectedData,
            at: location,
            policy: .dataProtectionPreferred
        )

        XCTAssertEqual(
            try GenericPasswordStore.read(at: location, policy: .fileBased),
            expectedData
        )
        XCTAssertEqual(
            try GenericPasswordStore.read(at: location, policy: .dataProtectionPreferred),
            expectedData
        )
    }

    private func makeLocation() -> GenericPasswordLocation {
        GenericPasswordLocation(
            service: "com.anyaicliremote.launcher.tests.\(UUID().uuidString)",
            account: "pairing-secret"
        )
    }

    private func deleteAllRepresentations(at location: GenericPasswordLocation) {
        try? GenericPasswordStore.delete(at: location, policy: .fileBased)
        try? GenericPasswordStore.delete(at: location, policy: .dataProtectionRequired)
    }
}
