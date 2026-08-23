import SwiftUI

struct ContentView: View {
  @ObservedObject var controller: DaemonController
  @ObservedObject var settings: LauncherSettings

  var body: some View {
    ZStack {
      LinearGradient(
        colors: [Color(nsColor: .windowBackgroundColor), Color.accentColor.opacity(0.055)],
        startPoint: .topLeading,
        endPoint: .bottomTrailing
      )
      .ignoresSafeArea()

      VStack(spacing: 18) {
        header
        HStack(alignment: .top, spacing: 18) {
          configurationCard
            .frame(maxWidth: .infinity)
          pairingCard
            .frame(width: 360)
        }
        logCard
          .frame(maxHeight: .infinity)
      }
      .padding(24)
    }
    .task { controller.activate() }
  }

  private var header: some View {
    HStack(spacing: 14) {
      ZStack {
        RoundedRectangle(cornerRadius: 13, style: .continuous)
          .fill(
            LinearGradient(
              colors: [.cyan, .blue], startPoint: .topLeading, endPoint: .bottomTrailing))
        Image(systemName: "antenna.radiowaves.left.and.right")
          .font(.system(size: 23, weight: .semibold))
          .foregroundStyle(.white)
      }
      .frame(width: 50, height: 50)
      .shadow(color: .blue.opacity(0.22), radius: 12, y: 5)

      VStack(alignment: .leading, spacing: 3) {
        Text("Any AI CLI Remote")
          .font(.system(size: 25, weight: .bold, design: .rounded))
        Text("配置服务，启动后用手机扫码即连")
          .font(.subheadline)
          .foregroundStyle(.secondary)
      }
      Spacer()
      StatusBadge(phase: controller.phase)
      Button(action: controller.performPrimaryAction) {
        Label(
          controller.showsStopAction ? "停止服务" : "启动服务",
          systemImage: controller.showsStopAction ? "stop.fill" : "play.fill"
        )
        .frame(minWidth: 96)
      }
      .buttonStyle(.borderedProminent)
      .controlSize(.large)
      .tint(controller.showsStopAction ? .red : .accentColor)
      .disabled(controller.phase == .stopping)
      .keyboardShortcut(.return, modifiers: [.command])
    }
  }

  private var configurationCard: some View {
    Card(title: "服务配置", systemImage: "slider.horizontal.3") {
      VStack(alignment: .leading, spacing: 14) {
        HStack(spacing: 14) {
          Field(title: "Daemon 端口") {
            TextField("请输入端口", value: $settings.daemonPort, format: .number.grouping(.never))
              .textFieldStyle(.roundedBorder)
          }
          Field(title: "Agent 端口") {
            TextField("请输入端口", value: $settings.agentPort, format: .number.grouping(.never))
              .textFieldStyle(.roundedBorder)
          }
          Field(title: "Bind 地址") {
            TextField("请输入绑定地址", text: $settings.bindAddress)
              .textFieldStyle(.roundedBorder)
          }
        }
        .disabled(!controller.configurationEditable)

        HStack(spacing: 14) {
          Field(title: "公网地址（可选）") {
            TextField("可选公网地址", text: $settings.publicHost)
              .textFieldStyle(.roundedBorder)
          }
        }
        .disabled(!controller.configurationEditable)
        HStack {
          Text(controller.configurationPath).font(.caption.monospaced()).textSelection(.enabled)
          Spacer()
          Button("保存配置") { Task { await controller.saveConfiguration() } }
            .disabled(!controller.configurationEditable)
          Button("重启服务", action: controller.restart)
            .disabled(!controller.showsStopAction || controller.phase == .stopping)
        }

        Divider()
        HStack(alignment: .firstTextBaseline, spacing: 9) {
          Image(
            systemName: controller.daemonExecutablePath.isEmpty
              ? "exclamationmark.triangle.fill" : "checkmark.seal.fill"
          )
          .foregroundStyle(controller.daemonExecutablePath.isEmpty ? .orange : .green)
          VStack(alignment: .leading, spacing: 3) {
            Text("Daemon 可执行文件").font(.caption).foregroundStyle(.secondary)
            Text(
              controller.daemonExecutablePath.isEmpty
                ? "未找到，请先构建后端" : controller.daemonExecutablePath
            )
            .font(.system(.caption, design: .monospaced))
            .lineLimit(2)
            .textSelection(.enabled)
          }
          Spacer()
          Button {
            controller.refreshDaemonLocation()
          } label: {
            Image(systemName: "arrow.clockwise")
          }
          .buttonStyle(.borderless)
          .help("重新定位 daemon")
        }
      }
    }
  }

  private var pairingCard: some View {
    Card(title: "手机配对", systemImage: "qrcode") {
      VStack(spacing: 12) {
        if !controller.isReachable || controller.pairingDeepLink.isEmpty {
          RoundedRectangle(cornerRadius: 14)
            .fill(.secondary.opacity(0.08))
            .overlay {
              VStack(spacing: 9) {
                Image(systemName: "qrcode")
                  .font(.system(size: 52, weight: .light))
                Text("启动后生成二维码").font(.subheadline)
              }
              .foregroundStyle(.secondary)
            }
            .frame(width: 210, height: 210)
        } else {
          QRCodeView(payload: controller.pairingDeepLink)
            .frame(width: 210, height: 210)
            .padding(8)
            .background(.white, in: RoundedRectangle(cornerRadius: 15, style: .continuous))
            .shadow(color: .black.opacity(0.09), radius: 10, y: 4)
        }

        Text(controller.phase.detail)
          .font(.caption)
          .foregroundStyle(.secondary)
          .multilineTextAlignment(.center)
          .lineLimit(2)

        if !controller.pairingURL.isEmpty {
          Text(controller.pairingURL)
            .font(.system(size: 10.5, design: .monospaced))
            .foregroundStyle(.secondary)
            .lineLimit(2)
            .textSelection(.enabled)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(8)
            .background(.secondary.opacity(0.06), in: RoundedRectangle(cornerRadius: 8))
        }

        HStack {
          Button("复制连接", action: controller.copyPairingURL)
            .disabled(controller.pairingURL.isEmpty)
          Button("复制扫码链接", action: controller.copyDeepLink)
            .disabled(controller.pairingDeepLink.isEmpty)
          Button {
            controller.openPairingURL()
          } label: {
            Image(systemName: "safari")
          }
          .help("在浏览器中打开")
          .disabled(controller.pairingURL.isEmpty)
        }
        .buttonStyle(.bordered)
      }
      .frame(maxWidth: .infinity)
    }
  }

  private var logCard: some View {
    Card(title: "运行日志", systemImage: "terminal") {
      VStack(spacing: 8) {
        HStack(spacing: 12) {
          if let health = controller.health {
            Label(
              health.hubUp == true ? "Hub 已连接" : "Hub 等待中",
              systemImage: health.hubUp == true ? "link" : "hourglass")
            Label("\(health.hubClients ?? 0) 个客户端", systemImage: "iphone")
          }
          Spacer()
          Button("复制", action: controller.copyLogs).buttonStyle(.borderless)
          Button("清空", action: controller.clearLogs).buttonStyle(.borderless)
        }
        .font(.caption)
        .foregroundStyle(.secondary)

        LogView(entries: controller.logs)
      }
    }
  }
}

private struct Field<Content: View>: View {
  let title: String
  @ViewBuilder let content: Content

  var body: some View {
    VStack(alignment: .leading, spacing: 7) {
      Text(title).font(.caption).foregroundStyle(.secondary)
      content
    }
    .frame(maxWidth: .infinity, alignment: .leading)
  }
}

private struct Card<Content: View>: View {
  let title: String
  let systemImage: String
  @ViewBuilder let content: Content

  var body: some View {
    VStack(alignment: .leading, spacing: 14) {
      Label(title, systemImage: systemImage)
        .font(.headline)
      content
    }
    .padding(18)
    .background(
      Color(nsColor: .controlBackgroundColor).opacity(0.92),
      in: RoundedRectangle(cornerRadius: 17, style: .continuous)
    )
    .overlay { RoundedRectangle(cornerRadius: 17).stroke(.primary.opacity(0.07)) }
    .shadow(color: .black.opacity(0.045), radius: 12, y: 5)
  }
}

private struct StatusBadge: View {
  let phase: DaemonPhase

  private var color: Color {
    switch phase {
    case .online: .green
    case .degraded, .starting, .stopping: .orange
    case .failed: .red
    case .stopped: .secondary
    }
  }

  var body: some View {
    HStack(spacing: 7) {
      Circle().fill(color).frame(width: 8, height: 8)
      Text(phase.title).font(.subheadline.weight(.medium))
    }
    .padding(.horizontal, 12)
    .padding(.vertical, 8)
    .background(color.opacity(0.1), in: Capsule())
  }
}

private struct LogView: View {
  let entries: [LogEntry]

  var body: some View {
    ScrollViewReader { proxy in
      ScrollView {
        LazyVStack(alignment: .leading, spacing: 4) {
          if entries.isEmpty {
            Text("等待日志…").foregroundStyle(.tertiary)
          }
          ForEach(entries) { entry in
            HStack(alignment: .top, spacing: 9) {
              Text(entry.date, format: .dateTime.hour().minute().second())
                .foregroundStyle(.tertiary)
              Text(entry.message)
                .foregroundStyle(.primary.opacity(0.88))
                .textSelection(.enabled)
            }
            .id(entry.id)
          }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(11)
      }
      .font(.system(size: 11.5, design: .monospaced))
      .background(
        Color(nsColor: .textBackgroundColor).opacity(0.7), in: RoundedRectangle(cornerRadius: 10)
      )
      .onChange(of: entries.last?.id) { _, identifier in
        guard let identifier else { return }
        withAnimation(.easeOut(duration: 0.15)) { proxy.scrollTo(identifier, anchor: .bottom) }
      }
    }
  }
}
