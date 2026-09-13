import SwiftUI

struct CompletionSoundSection: View {
    @Environment(AppModel.self) private var model

    @State private var isImportingSound = false
    @State private var importErrorMessage: String?

    var body: some View {
        Section("Completion sounds") {
            soundControlCard(
                title: "Task completion",
                subtitle: "Played when a task is marked complete from any surface.",
                isEnabled: taskCompletionEnabledBinding,
                choice: taskCompletionChoiceBinding
            )

            soundControlCard(
                title: "Event completion",
                subtitle: "Played when an event is marked done and dismissed from Calendar.",
                isEnabled: eventCompletionEnabledBinding,
                choice: eventCompletionChoiceBinding
            )

            HStack {
                Button {
                    isImportingSound = true
                } label: {
                    Label("Import sound…", systemImage: "square.and.arrow.down")
                }

                Spacer(minLength: 12)

                Text("\(model.settings.customCompletionSounds.count) imported")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .monospacedDigit()
            }

            if model.settings.customCompletionSounds.isEmpty == false {
                importedLibrary
            }

            Text("Choose separate sounds for tasks and events, preview them before selecting, or import your own audio. Imported sounds are copied into Hot Cross Buns so playback keeps working even if the original file moves.")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        }
        .fileImporter(
            isPresented: $isImportingSound,
            allowedContentTypes: CompletionSoundLibrary.supportedAudioTypes,
            allowsMultipleSelection: false
        ) { result in
            handleImport(result)
        }
        .alert("Couldn't import sound", isPresented: Binding(
            get: { importErrorMessage != nil },
            set: { if $0 == false { importErrorMessage = nil } }
        )) {
            Button("OK", role: .cancel) {
                importErrorMessage = nil
            }
        } message: {
            Text(importErrorMessage ?? "")
        }
    }

    @ViewBuilder
    private func soundControlCard(
        title: String,
        subtitle: String,
        isEnabled: Binding<Bool>,
        choice: Binding<CompletionSoundChoice>
    ) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Toggle(isOn: isEnabled) {
                VStack(alignment: .leading, spacing: 3) {
                    Text(title)
                    Text(subtitle)
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
            }

            if isEnabled.wrappedValue {
                HStack(spacing: 10) {
                    Picker("Sound", selection: choice) {
                        systemSoundChoices
                        if model.settings.customCompletionSounds.isEmpty == false {
                            Divider()
                            customSoundChoices
                        }
                    }
                    .pickerStyle(.menu)
                    .labelsHidden()

                    Button {
                        CompletionSoundPlayer.preview(choice.wrappedValue, customAssets: model.settings.customCompletionSounds)
                    } label: {
                        Label("Preview", systemImage: "play.fill")
                    }
                    .buttonStyle(.borderless)

                    Spacer(minLength: 0)

                    Text(displayName(for: choice.wrappedValue))
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
        }
        .hcbScaledPadding(.vertical, 4)
    }

    private var systemSoundChoices: some View {
        Group {
            ForEach(CompletionSoundLibrary.builtInSoundNames, id: \.self) { soundName in
                Text(soundName)
                    .tag(CompletionSoundChoice.system(soundName))
            }
        }
    }

    private var customSoundChoices: some View {
        Group {
            ForEach(model.settings.customCompletionSounds) { asset in
                Text(asset.displayName)
                    .tag(CompletionSoundChoice.custom(asset.id))
            }
        }
    }

    @ViewBuilder
    private var importedLibrary: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Imported library")
                .hcbFont(.caption, weight: .semibold)
                .foregroundStyle(.secondary)

            ForEach(model.settings.customCompletionSounds) { asset in
                HStack(spacing: 10) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(asset.displayName)
                            .lineLimit(1)
                        Text("Imported \(asset.importedAt.formatted(date: .abbreviated, time: .shortened))")
                            .hcbFont(.caption2)
                            .foregroundStyle(.secondary)
                    }

                    Spacer(minLength: 8)

                    Button {
                        CompletionSoundPlayer.preview(.custom(asset.id), customAssets: model.settings.customCompletionSounds)
                    } label: {
                        Label("Preview", systemImage: "play.fill")
                    }
                    .buttonStyle(.borderless)

                    Button(role: .destructive) {
                        model.deleteCustomCompletionSound(asset.id)
                    } label: {
                        Label("Delete", systemImage: "trash")
                    }
                    .buttonStyle(.borderless)
                }
            }
        }
        .hcbScaledPadding(.top, 4)
    }

    private func displayName(for choice: CompletionSoundChoice) -> String {
        switch choice.source {
        case .system:
            return choice.identifier
        case .custom:
            guard
                let assetID = choice.customAssetID,
                let asset = model.settings.customCompletionSounds.first(where: { $0.id == assetID })
            else {
                return "Imported sound"
            }
            return asset.displayName
        }
    }

    private func handleImport(_ result: Result<[URL], Error>) {
        do {
            guard let url = try result.get().first else { return }
            _ = try model.importCustomCompletionSound(from: url)
        } catch {
            importErrorMessage = error.localizedDescription
        }
    }

    private var taskCompletionEnabledBinding: Binding<Bool> {
        Binding(
            get: { model.settings.enableTaskCompletionSound },
            set: { model.setTaskCompletionSoundEnabled($0) }
        )
    }

    private var eventCompletionEnabledBinding: Binding<Bool> {
        Binding(
            get: { model.settings.enableEventCompletionSound },
            set: { model.setEventCompletionSoundEnabled($0) }
        )
    }

    private var taskCompletionChoiceBinding: Binding<CompletionSoundChoice> {
        Binding(
            get: { model.settings.taskCompletionSoundChoice },
            set: { model.setTaskCompletionSoundChoice($0) }
        )
    }

    private var eventCompletionChoiceBinding: Binding<CompletionSoundChoice> {
        Binding(
            get: { model.settings.eventCompletionSoundChoice },
            set: { model.setEventCompletionSoundChoice($0) }
        )
    }
}
