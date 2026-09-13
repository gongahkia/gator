import SwiftUI

extension View {
    @ViewBuilder
    func enableWritingTools() -> some View {
        if #available(macOS 15.1, *) {
            self.writingToolsBehavior(.complete)
        } else {
            self
        }
    }
}
