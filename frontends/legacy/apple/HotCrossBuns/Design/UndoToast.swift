import SwiftUI

struct UndoToast: View {
    @Environment(AppModel.self) private var model
    @Environment(\.hcbReduceMotion) private var reduceMotion

    var body: some View {
        VStack {
            Spacer(minLength: 0)
            if let action = model.undoable {
                content(action: action)
                    .transition(HCBMotion.transition(.move(edge: .bottom).combined(with: .opacity), reduceMotion: reduceMotion))
                    .task(id: model.undoActionToken) {
                        try? await Task.sleep(for: .seconds(6))
                        if model.undoable == action {
                            model.clearUndo()
                        }
                    }
            }
        }
        .animation(HCBMotion.animation(.easeOut(duration: 0.16), reduceMotion: reduceMotion), value: model.undoActionToken)
        .allowsHitTesting(model.undoable != nil)
    }

    private func content(action: UndoableAction) -> some View {
        HStack(spacing: 12) {
            Image(systemName: action.sfSymbol)
                .foregroundStyle(AppColor.moss)
            Text(action.summary)
                .hcbFont(.subheadline, weight: .medium)
                .lineLimit(1)
            Spacer(minLength: 16)
            Button("Undo") {
                Task { await model.performUndo() }
            }
            .buttonStyle(.bordered)
            Button {
                model.clearUndo()
            } label: {
                Image(systemName: "xmark")
                    .hcbFont(.caption)
            }
            .buttonStyle(.borderless)
            .accessibilityLabel("Dismiss")
        }
        .hcbScaledPadding(.horizontal, 16)
        .hcbScaledPadding(.vertical, 12)
        .background(
            RoundedRectangle(cornerRadius: 10)
                .fill(.regularMaterial)
        )
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .strokeBorder(AppColor.cardStroke, lineWidth: 0.8)
        )
        .shadow(color: .black.opacity(0.15), radius: 10, x: 0, y: 3)
        .hcbScaledPadding(18)
    }
}
