import XCTest

@testable import AnyAICLIRemoteCore
@testable import AnyAICLIRemoteFeature

final class InteractionNotificationMapperTests: XCTestCase {
  private let identity = SessionIdentity(providerID: "provider-a", sessionID: "session-a")

  private func payload(
    method: String = ACPWire.Method.interactionRequest,
    id: Any? = 11,
    params: [String: Any]? = nil
  ) -> [String: Any] {
    var envelope: [String: Any] = ["method": method]
    if let id { envelope["id"] = id }
    envelope["params"] = params ?? [
      "providerId": "provider-a",
      "sessionId": "session-a",
      "toolCallId": "tool-1",
      "kind": "ask_question",
      "questions": [["question": "Pick one", "options": [["label": "Yes"]], "multiSelect": false]]
    ]
    return envelope
  }

  func testMapsMatchingSession() {
    guard case let .interaction(request)? = ChatNotificationMapper.map(
      payload: payload(), selectedSessionID: identity
    ) else { return XCTFail("expected interaction") }
    XCTAssertEqual(request.rpcID, 11)
    XCTAssertEqual(request.kind, .askQuestion)
    XCTAssertEqual(request.toolCallID, "tool-1")
    XCTAssertEqual(request.questions.first?.question, "Pick one")
  }

  func testRejectsWrongSessionAndSelectedNil() {
    let wrongProvider = payload(params: [
      "providerId": "other", "sessionId": "session-a", "toolCallId": "tool-1", "kind": "ask_question"
    ])
    XCTAssertNil(ChatNotificationMapper.map(payload: wrongProvider, selectedSessionID: identity))
    let wrongSession = payload(params: [
      "providerId": "provider-a", "sessionId": "other", "toolCallId": "tool-1", "kind": "ask_question"
    ])
    XCTAssertNil(ChatNotificationMapper.map(payload: wrongSession, selectedSessionID: identity))
    XCTAssertNil(ChatNotificationMapper.map(payload: payload(), selectedSessionID: nil))
  }

  func testRejectsMalformedInteractionRequests() {
    let malformed: [[String: Any]] = [
      payload(id: nil),
      payload(id: "11"),
      ["method": ACPWire.Method.interactionRequest, "id": 11],
      payload(params: [
        "providerId": "provider-a", "sessionId": "session-a", "kind": "ask_question"
      ]),
      payload(params: [
        "providerId": "provider-a", "sessionId": "session-a", "toolCallId": "tool-1", "kind": "other"
      ])
    ]
    for value in malformed {
      XCTAssertNil(ChatNotificationMapper.map(payload: value, selectedSessionID: identity))
    }
  }

  func testProviderAskUserQuestionMethodIsNotMapped() {
    XCTAssertNil(
      ChatNotificationMapper.map(
        payload: payload(
          method: "_x.ai/ask_user_question",
          params: [
            "providerId": "provider-a", "sessionId": "session-a", "toolCallId": "tool-1",
            "kind": "ask_question",
            "questions": [["question": "Pick one", "options": [["label": "Yes"]], "multiSelect": false]]
          ]
        ),
        selectedSessionID: identity
      )
    )
  }
}
