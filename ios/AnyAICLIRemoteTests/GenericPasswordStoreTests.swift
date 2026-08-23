import Foundation
import XCTest
@testable import AnyAICLIRemote

final class GenericPasswordStoreTests: XCTestCase {
    func testGenericPasswordRoundTripUpdateAndIdempotentDelete() throws {
        let location = GenericPasswordLocation(
            service: "com.anyaicliremote.tests.\(UUID().uuidString)",
            account: "round-trip"
        )
        defer { try? GenericPasswordStore.delete(at: location) }

        XCTAssertNil(try GenericPasswordStore.read(at: location))

        let initialData = Data("first".utf8)
        try GenericPasswordStore.save(initialData, at: location)
        XCTAssertEqual(try GenericPasswordStore.read(at: location), initialData)

        let updatedData = Data("second".utf8)
        try GenericPasswordStore.save(updatedData, at: location)
        XCTAssertEqual(try GenericPasswordStore.read(at: location), updatedData)

        try GenericPasswordStore.delete(at: location)
        try GenericPasswordStore.delete(at: location)
        XCTAssertNil(try GenericPasswordStore.read(at: location))
    }
}
