import XCTest

@testable import AnyAICLIRemoteCore

final class InteractionAnswerCodecTests: XCTestCase {
  func testAcceptEncodesAnswersAsObjectKeyedByIndex() {
    let result = InteractionAnswerCodec.result(.accept(answers: ["1": ["进程内 LRU"]], annotations: [:]))
    XCTAssertEqual(result["outcome"] as? String, "accepted")
    XCTAssertEqual(result["answers"] as? [String: [String]], ["1": ["进程内 LRU"]])
  }

  func testAcceptEncodesTrimmedAnnotationsAndOtherForCustomOnly() {
    let result = InteractionAnswerCodec.result(.accept(
      answers: ["0": ["Blue"], "1": ["Other"]],
      annotations: ["0": InteractionAnnotation(notes: "  移动端  "), "1": InteractionAnnotation(notes: "  自定义  ")]
    ))
    let answers = result["answers"] as? [String: [String]]
    XCTAssertEqual(answers, ["0": ["Blue"], "1": ["Other"]])
    let annotations = result["annotations"] as? [String: [String: String]]
    XCTAssertEqual(annotations, ["0": ["notes": "移动端"], "1": ["notes": "自定义"]])
  }

  func testAcceptOmitsEmptyAnnotationsAndAskCancelHasOutcomeOnly() {
    let result = InteractionAnswerCodec.result(.accept(
      answers: ["0": ["Other"]],
      annotations: ["0": InteractionAnnotation(notes: "  ")]
    ))
    XCTAssertNil(result["annotations"])
    XCTAssertEqual(InteractionAnswerCodec.result(.cancelAsk)["outcome"] as? String, "cancelled")
  }

  func testApproveHasNoFeedback() {
    let result = InteractionAnswerCodec.result(.approve)
    XCTAssertEqual(result["outcome"] as? String, "approved")
    XCTAssertNil(result["feedback"])
  }

  func testCancelOmitsBlankFeedbackButKeepsRealFeedback() {
    let blank = InteractionAnswerCodec.result(.cancel(feedback: "  "))
    XCTAssertEqual(blank["outcome"] as? String, "cancelled")
    XCTAssertNil(blank["feedback"])
    let real = InteractionAnswerCodec.result(.cancel(feedback: "加错误处理"))
    XCTAssertEqual(real["outcome"] as? String, "cancelled")
    XCTAssertEqual(real["feedback"] as? String, "加错误处理")
  }

  func testCancelTrimsFeedback() {
    XCTAssertEqual(InteractionAnswerCodec.result(.cancel(feedback: "  反馈  "))["feedback"] as? String, "反馈")
  }

  func testAbandonEncodesOutcomeOnly() {
    let result = InteractionAnswerCodec.result(.abandon)
    XCTAssertEqual(result["outcome"] as? String, "abandoned")
    XCTAssertNil(result["feedback"])
  }

  func testChatAndSkipAlwaysIncludePartialAnswersObject() {
    let chat = InteractionAnswerCodec.result(.chatAbout(partialAnswers: ["0": "看情况"]))
    XCTAssertEqual(chat["outcome"] as? String, "chat_about_this")
    XCTAssertEqual(chat["partialAnswers"] as? [String: String], ["0": "看情况"])
    let skip = InteractionAnswerCodec.result(.skipInterview(partialAnswers: [:]))
    XCTAssertEqual(skip["outcome"] as? String, "skip_interview")
    let partialAnswers = skip["partialAnswers"] as? [String: String]
    XCTAssertNotNil(partialAnswers)
    XCTAssertEqual(partialAnswers, [:])
  }
}
