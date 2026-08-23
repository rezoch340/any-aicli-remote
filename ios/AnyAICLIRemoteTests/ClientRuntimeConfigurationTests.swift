import XCTest
@testable import AnyAICLIRemote

final class ClientRuntimeConfigurationTests: XCTestCase {
    func testDefaultDurations() {
        let configuration = ClientRuntimeConfiguration()
        XCTAssertEqual(configuration.initializeTimeout, 20)
        XCTAssertEqual(configuration.rpcTimeout, 120)
        XCTAssertEqual(configuration.sessionLoadTimeout, 90)
        XCTAssertEqual(configuration.sessionCreateTimeout, 60)
        XCTAssertEqual(configuration.healthRequestTimeout, 3)
        XCTAssertEqual(configuration.healthPollingInterval, 5)
    }

    func testCustomDurationsArePreserved() {
        let configuration = ClientRuntimeConfiguration(
            initializeTimeout: 11, rpcTimeout: 22, sessionLoadTimeout: 33,
            sessionCreateTimeout: 44, healthRequestTimeout: 2, healthPollingInterval: 7
        )
        XCTAssertEqual(configuration.initializeTimeout, 11)
        XCTAssertEqual(configuration.rpcTimeout, 22)
        XCTAssertEqual(configuration.sessionLoadTimeout, 33)
        XCTAssertEqual(configuration.sessionCreateTimeout, 44)
        XCTAssertEqual(configuration.healthRequestTimeout, 2)
        XCTAssertEqual(configuration.healthPollingInterval, 7)
    }
}
