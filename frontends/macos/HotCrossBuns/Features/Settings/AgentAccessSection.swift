import AppKit
import SwiftUI

struct AgentAccessSection: View {
    @Environment(AppModel.self) private var model
    @State private var statusMessage: String?

    var body: some View {
        Section("Agent access") {
            Toggle("Local MCP server", isOn: enabledBinding)

            Picker("Permission mode", selection: permissionModeBinding) {
                ForEach(MCPPermissionMode.allCases) { mode in
                    Text(mode.title).tag(mode)
                }
            }
            .pickerStyle(.menu)
            .disabled(model.settings.mcpServerEnabled == false)

            Text(model.settings.mcpPermissionMode.detail)
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)

            HStack {
                Text("Port")
                TextField("Port", value: portBinding, format: .number)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 96)
                    .multilineTextAlignment(.trailing)
            }
            .disabled(model.settings.mcpServerEnabled == false)

            Label(model.mcpServerStatus.title, systemImage: statusImageName)
                .foregroundStyle(statusForegroundStyle)
            Text(model.mcpServerStatus.detail)
                .hcbFont(.caption)
                .foregroundStyle(.secondary)

            VStack(alignment: .leading, spacing: 8) {
                HStack {
                    Label("Recent MCP activity", systemImage: "waveform.path.ecg")
                        .hcbFont(.caption)
                        .fontWeight(.semibold)
                    Spacer()
                    Button {
                        model.clearMCPRecentActivity()
                    } label: {
                        Label("Clear", systemImage: "xmark.circle")
                    }
                    .buttonStyle(.borderless)
                    .disabled(model.mcpRecentActivity.isEmpty)
                }

                if model.mcpRecentActivity.isEmpty {
                    Text("No MCP activity this launch.")
                        .hcbFont(.caption2)
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(Array(model.mcpRecentActivity.prefix(6))) { entry in
                        HStack(alignment: .firstTextBaseline, spacing: 8) {
                            Image(systemName: entry.outcome.symbolName)
                                .foregroundStyle(outcomeForegroundStyle(entry.outcome))
                                .frame(width: 16)
                            VStack(alignment: .leading, spacing: 2) {
                                HStack(spacing: 6) {
                                    Text(entry.title)
                                        .hcbFont(.caption)
                                        .lineLimit(1)
                                    Text(entry.outcome.title)
                                        .hcbFont(.caption2)
                                        .foregroundStyle(outcomeForegroundStyle(entry.outcome))
                                }
                                Text("\(entry.detail) - \(entry.client) - \(entry.timestamp, style: .relative)")
                                    .hcbFont(.caption2)
                                    .foregroundStyle(.secondary)
                                    .lineLimit(2)
                            }
                        }
                    }
                }
            }
            .padding(.vertical, 4)

            HStack {
                Button {
                    copyClientConfiguration()
                } label: {
                    Label("Copy client config", systemImage: "doc.on.doc")
                }
                .disabled(model.settings.mcpServerEnabled == false)

                Button(role: .destructive) {
                    resetToken()
                } label: {
                    Label("Reset token", systemImage: "arrow.clockwise")
                }
            }

            Text("Local MCP clients with the bearer token can read exposed Hot Cross Buns data and request actions according to the permission mode. Google OAuth tokens, cache encryption keys, and raw credentials are never returned by MCP tools.")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)

            if let statusMessage {
                Text(statusMessage)
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var enabledBinding: Binding<Bool> {
        Binding(
            get: { model.settings.mcpServerEnabled },
            set: { model.setMCPServerEnabled($0) }
        )
    }

    private var permissionModeBinding: Binding<MCPPermissionMode> {
        Binding(
            get: { model.settings.mcpPermissionMode },
            set: { model.setMCPPermissionMode($0) }
        )
    }

    private var portBinding: Binding<Int> {
        Binding(
            get: { model.settings.mcpPort },
            set: { model.setMCPPort($0) }
        )
    }

    private var statusImageName: String {
        switch model.mcpServerStatus {
        case .stopped:
            "power"
        case .running:
            "network"
        case .failed:
            "exclamationmark.triangle"
        }
    }

    private var statusForegroundStyle: AnyShapeStyle {
        switch model.mcpServerStatus {
        case .running:
            AnyShapeStyle(AppColor.moss)
        case .failed:
            AnyShapeStyle(AppColor.ember)
        case .stopped:
            AnyShapeStyle(Color.secondary)
        }
    }

    private func outcomeForegroundStyle(_ outcome: MCPActivityOutcome) -> AnyShapeStyle {
        switch outcome {
        case .applied, .succeeded, .dryRun:
            AnyShapeStyle(AppColor.moss)
        case .confirmationRequired, .rateLimited:
            AnyShapeStyle(Color.orange)
        case .denied, .invalid, .failed:
            AnyShapeStyle(AppColor.ember)
        }
    }

    private func copyClientConfiguration() {
        let pasteboard = NSPasteboard.general
        pasteboard.clearContents()
        pasteboard.setString(model.mcpClientConfigurationJSON(), forType: .string)
        statusMessage = "Client config copied."
    }

    private func resetToken() {
        if model.resetMCPBearerToken() != nil {
            statusMessage = "MCP token reset. Update any client config that used the old token."
        } else {
            statusMessage = model.lastMutationError ?? "Could not reset MCP token."
        }
    }
}
