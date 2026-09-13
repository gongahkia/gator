import SwiftUI

struct OpenAtLoginSection: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        Section("Startup") {
            Toggle("Open Hot Cross Buns at login", isOn: openAtLoginBinding)

            Text("Starts the app automatically when you sign in to this Mac.")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)

            if let error = model.loginItemError {
                Text(error)
                    .hcbFont(.caption2)
                    .foregroundStyle(.red)
            }
        }
        .onAppear {
            model.refreshOpenAtLoginStatus()
        }
    }

    private var openAtLoginBinding: Binding<Bool> {
        Binding(
            get: { model.opensAtLogin },
            set: { model.setOpenAtLogin($0) }
        )
    }
}
