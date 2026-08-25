import SwiftUI

/// Thin transient bar: an interaction-mode badge (plan/yolo) and a passing status
/// notice (retry / auto model switch). Display-only; empty values render nothing.
struct SessionStatusBar: View {
  let mode: String
  let notice: String

  var body: some View {
    let badge = modeBadge(mode)
    if badge.isEmpty && notice.isEmpty {
      EmptyView()
    } else {
      HStack(spacing: 8) {
        if !badge.isEmpty {
          Text(badge)
            .font(.caption2.weight(.semibold))
            .padding(.horizontal, 8)
            .padding(.vertical, 2)
            .background(Color.accentColor.opacity(0.16), in: RoundedRectangle(cornerRadius: 6))
            .accessibilityIdentifier("session-mode-badge")
        }
        if !notice.isEmpty {
          Text(notice)
            .font(.caption2)
            .foregroundStyle(.secondary)
            .accessibilityIdentifier("session-status-notice")
        }
        Spacer(minLength: 0)
      }
      .padding(.horizontal, 12)
      .padding(.vertical, 4)
    }
  }

  // Only surface a badge for modes worth flagging; plain "normal" needs no chrome.
  private func modeBadge(_ mode: String) -> String {
    switch mode.lowercased() {
    case "plan": return "计划模式"
    case "yolo": return "YOLO 模式"
    default: return ""
    }
  }
}
