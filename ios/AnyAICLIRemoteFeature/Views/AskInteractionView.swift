import AnyAICLIRemoteCore
import SwiftUI

struct AskInteractionView: View {
  let interaction: PendingInteraction
  let onAnswer: (InteractionAnswer) -> Void

  @State private var selections: [Int: Set<String>] = [:]
  @State private var customAnswers: [Int: String] = [:]
  @FocusState private var focusedQuestion: Int?

  var body: some View {
    VStack(alignment: .leading, spacing: 0) {
      Text("助手需要你的确认")
        .font(.title3.weight(.semibold))
        .padding(.horizontal, 20)
        .padding(.top, 16)
        .accessibilityIdentifier("interaction-ask-sheet")

      ScrollView {
        VStack(alignment: .leading, spacing: 16) {
          ForEach(Array(interaction.questions.enumerated()), id: \.offset) { questionIndex, question in
            questionView(question, at: questionIndex)
          }
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 16)
      }

      Divider()
      actionButtons
        .padding(.horizontal, 20)
        .padding(.vertical, 12)
    }
  }

  private var actionButtons: some View {
    VStack(spacing: 8) {
      if interaction.mode == "plan" {
        HStack(spacing: 12) {
          Button("先聊一下") { onAnswer(.chatAbout(partialAnswers: partialAnswers)) }
            .disabled(partialAnswers.isEmpty)
            .accessibilityIdentifier("interaction-chat-about")
          Button("跳过") { onAnswer(.skipInterview(partialAnswers: partialAnswers)) }
            .accessibilityIdentifier("interaction-skip")
        }
      }
      HStack(spacing: 12) {
        Button("取消") { onAnswer(.cancelAsk) }
          .accessibilityIdentifier("interaction-cancel")
        Spacer()
        Button("提交") { onAnswer(.accept(answers: acceptedAnswers, annotations: annotations)) }
          .disabled(!canSubmit)
          .accessibilityIdentifier("interaction-submit")
      }
    }
  }

  private var canSubmit: Bool {
    !acceptedAnswers.isEmpty
  }

  private var partialAnswers: [String: String] {
    Dictionary(uniqueKeysWithValues: interaction.questions.enumerated().compactMap { index, question in
      let labels = question.options.compactMap { selections[index, default: []].contains($0.label) ? $0.label : nil }
      let custom = customAnswers[index]?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
      guard !labels.isEmpty || !custom.isEmpty else { return nil }
      return (String(index), labels.isEmpty ? "Other" : labels.joined(separator: ", "))
    })
  }

  private var acceptedAnswers: [String: [String]] {
    Dictionary(uniqueKeysWithValues: interaction.questions.enumerated().compactMap { questionIndex, question in
      let selectedLabels = question.options.compactMap { option in
        selections[questionIndex, default: []].contains(option.label) ? option.label : nil
      }
      let customAnswer = customAnswers[questionIndex]?
        .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
      let answers = selectedLabels + (selectedLabels.isEmpty && !customAnswer.isEmpty ? ["Other"] : [])
      guard !answers.isEmpty else { return nil }
      return (String(questionIndex), answers)
    })
  }

  private var annotations: [String: InteractionAnnotation] {
    Dictionary(uniqueKeysWithValues: customAnswers.compactMap { index, answer in
      let trimmed = answer.trimmingCharacters(in: .whitespacesAndNewlines)
      guard !trimmed.isEmpty else { return nil }
      return (String(index), InteractionAnnotation(notes: trimmed))
    })
  }

  private func questionView(_ question: InteractionQuestion, at questionIndex: Int) -> some View {
    VStack(alignment: .leading, spacing: 8) {
      Text(question.question)
        .font(.body.weight(.medium))
        .accessibilityIdentifier("interaction-question-\(questionIndex)")

      ForEach(Array(question.options.enumerated()), id: \.offset) { optionIndex, option in
        optionButton(option, question: question, questionIndex: questionIndex, optionIndex: optionIndex)
      }

      TextField(
        "其他回答 / 输入提示词",
        text: Binding(
          get: { customAnswers[questionIndex, default: ""] },
          set: { customAnswers[questionIndex] = $0 }
        ),
        axis: .vertical
      )
      .lineLimit(1...4)
      .textFieldStyle(.roundedBorder)
      .accessibilityIdentifier("interaction-custom-answer-\(questionIndex)")
      .focused($focusedQuestion, equals: questionIndex)
    }
  }

  private func optionButton(
    _ option: InteractionOption,
    question: InteractionQuestion,
    questionIndex: Int,
    optionIndex: Int
  ) -> some View {
    let isSelected = selections[questionIndex, default: []].contains(option.label)
    return Button {
      updateSelection(
        questionIndex: questionIndex,
        optionLabel: option.label,
        allowsMultiple: question.multiSelect
      )
      focusedQuestion = nil
    } label: {
      HStack(alignment: .top, spacing: 10) {
        VStack(alignment: .leading, spacing: 3) {
          Text(option.label)
            .font(.body.weight(.medium))
            .frame(maxWidth: .infinity, alignment: .leading)
          if !option.description.isEmpty {
            Text(option.description)
              .font(.caption)
              .foregroundStyle(.secondary)
          }
        }
        Image(systemName: isSelected ? "checkmark.circle.fill" : "circle")
          .foregroundStyle(isSelected ? Color.cyan : Color.secondary)
      }
      .padding(12)
      .background(isSelected ? Color.cyan.opacity(0.16) : Color.clear, in: RoundedRectangle(cornerRadius: 10))
      .overlay(RoundedRectangle(cornerRadius: 10).stroke(Color.secondary.opacity(0.35)))
      .contentShape(Rectangle())
    }
    .buttonStyle(.plain)
    .accessibilityIdentifier("interaction-option-\(questionIndex)-\(optionIndex)")
    .accessibilityValue(isSelected ? "selected" : "unselected")
    .accessibilityAddTraits(isSelected ? .isSelected : [])
  }

  private func updateSelection(questionIndex: Int, optionLabel: String, allowsMultiple: Bool) {
    if allowsMultiple {
      if selections[questionIndex, default: []].contains(optionLabel) {
        selections[questionIndex, default: []].remove(optionLabel)
      } else {
        selections[questionIndex, default: []].insert(optionLabel)
      }
    } else {
      selections[questionIndex] = [optionLabel]
    }
  }
}
