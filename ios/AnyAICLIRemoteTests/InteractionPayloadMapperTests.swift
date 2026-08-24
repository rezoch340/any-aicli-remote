import XCTest

@testable import AnyAICLIRemoteCore

final class InteractionPayloadMapperTests: XCTestCase {
  private func envelope(id: Any = 7, method: String = ACPWire.Method.interactionRequest, params: [String: Any])
    -> [String: Any] {
    ["jsonrpc": "2.0", "id": id, "method": method, "params": params]
  }

  private func baseParams(_ extras: [String: Any] = [:]) -> [String: Any] {
    var params: [String: Any] = [
      "providerId": "provider-a", "sessionId": "session-a", "toolCallId": "tool-1",
      "kind": "ask_question"
    ]
    extras.forEach { params[$0.key] = $0.value }
    return params
  }

  func testAskMappingUsesQuestionNotTextAndPreservesOptions() {
    let result = InteractionPayloadMapper.request(from: envelope(params: baseParams([
      "questions": [[
        "text": "ignored",
        "question": "Pick one",
        "multiSelect": true,
        "options": [["label": "Yes", "description": "go"]]
      ]]
    ])))
    XCTAssertEqual(result?.kind, .askQuestion)
    XCTAssertEqual(result?.rpcID, 7)
    XCTAssertEqual(result?.questions.first?.question, "Pick one")
    XCTAssertEqual(result?.questions.first?.options.first?.label, "Yes")
    XCTAssertEqual(result?.questions.first?.options.first?.description, "go")
    XCTAssertEqual(result?.questions.first?.multiSelect, true)
  }

  func testExitPlanMappingReadsPlanContentAndMode() {
    let result = InteractionPayloadMapper.request(from: envelope(params: [
      "providerId": "provider-a", "sessionId": "session-a", "toolCallId": "plan-1",
      "kind": "exit_plan", "planContent": "# Plan", "mode": "plan"
    ]))
    XCTAssertEqual(result?.kind, .exitPlan)
    XCTAssertEqual(result?.planContent, "# Plan")
    XCTAssertEqual(result?.mode, "plan")
    XCTAssertEqual(result?.toolCallID, "plan-1")
  }

  func testUnknownKindFailsClosed() {
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: baseParams(["kind": "other"]))))
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: baseParams(["kind": "ask_user"]))))
  }

  func testMissingToolCallIdFailsClosed() {
    var params = baseParams(["questions": [["question": "q"]]])
    params.removeValue(forKey: "toolCallId")
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: params)))
    params["toolCallId"] = "  "
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: params)))
  }

  func testMissingProviderOrSessionFailsClosed() {
    for key in ["providerId", "sessionId"] {
      var params = baseParams(["questions": [["question": "q"]]])
      params.removeValue(forKey: key)
      XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: params)), key)
      params[key] = "  "
      XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: params)), key)
    }
  }

  func testExtraKeysStillMap() {
    var payload = envelope(params: baseParams([
      "questions": [["question": "q", "options": [["label": "A"]], "multiSelect": false]],
      "_x.ai/private": true
    ]))
    payload["_x.ai/extra"] = "yes"
    let result = InteractionPayloadMapper.request(from: payload)
    XCTAssertEqual(result?.questions.first?.question, "q")
    XCTAssertEqual(result?.questions.first?.options.first?.label, "A")
  }

  func testAskMissingOrEmptyQuestionsAndLabelsFailClosed() {
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: baseParams())))
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: baseParams([
      "questions": [[String: Any]]()
    ]))))
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: baseParams([
      "questions": [["question": "q", "options": [["description": "only"]]]]
    ]))))
  }

  func testStringRpcIdFailsClosed() {
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(
      id: "7", params: baseParams(["questions": [["question": "q"]]]))))
  }

  func testInvalidIDsFailClosed() {
    for id: Any in [-1, NSNumber(value: true), "7", 1.5, NSNumber(value: UInt64.max)] {
      XCTAssertNil(InteractionPayloadMapper.request(from: envelope(
        id: id, params: baseParams(["questions": [["question": "q"]]]))))
    }
  }

  func testBlankQuestionAndMalformedOptionsFailClosed() {
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: baseParams([
      "questions": [["question": "  "]]
    ]))))
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: baseParams([
      "questions": [[String: Any]()]
    ]))))
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: baseParams([
      "questions": "bad"
    ]))))
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: baseParams([
      "questions": [["question": "q", "options": "bad"]]
    ]))))
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(params: baseParams([
      "questions": [["question": "q", "options": [["label": "  "]]]]
    ]))))
  }

  func testExitPlanAllowsEmptyContentAndPreservesModeAndTrimsIdentity() {
    let result = InteractionPayloadMapper.request(from: envelope(params: [
      "providerId": "  provider-a  ", "sessionId": " session-a ", "toolCallId": " plan ",
      "kind": "exit_plan", "mode": " plan "
    ]))
    XCTAssertEqual(result?.sessionIdentity.providerID, "provider-a")
    XCTAssertEqual(result?.sessionIdentity.sessionID, "session-a")
    XCTAssertEqual(result?.mode, " plan ")
  }

  func testWrongAndPrivateMethodsFailClosed() {
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(
      method: "_x.ai/private", params: baseParams(["questions": [["question": "q"]]]))))
    XCTAssertNil(InteractionPayloadMapper.request(from: envelope(
      method: "wrong", params: baseParams(["questions": [["question": "q"]]]))))
  }

  func testMissingOptionsAndNonBooleanMultiSelectFailClosed() {
    let missingOptions = InteractionPayloadMapper.request(from: envelope(params: baseParams([
      "questions": [["question": "q", "multiSelect": false]]
    ])))
    XCTAssertNil(missingOptions)
    let wrongMultiSelect = InteractionPayloadMapper.request(from: envelope(params: baseParams([
      "questions": [["question": "q", "options": [], "multiSelect": "true"]]
    ])))
    XCTAssertNil(wrongMultiSelect)
  }

  func testWrongDescriptionAndOptionalTopLevelTypesFailClosed() {
    let wrongDescription = InteractionPayloadMapper.request(from: envelope(params: baseParams([
      "questions": [["question": "q", "options": [["label": "A", "description": 7]], "multiSelect": false]]
    ])))
    XCTAssertNil(wrongDescription)
    let wrongMode = InteractionPayloadMapper.request(from: envelope(params: [
      "providerId": "provider-a", "sessionId": "session-a", "toolCallId": "plan-1",
      "kind": "exit_plan", "mode": 7
    ]))
    XCTAssertNil(wrongMode)
    let wrongPlanContent = InteractionPayloadMapper.request(from: envelope(params: [
      "providerId": "provider-a", "sessionId": "session-a", "toolCallId": "plan-1",
      "kind": "exit_plan", "planContent": 7
    ]))
    XCTAssertNil(wrongPlanContent)
  }
}
