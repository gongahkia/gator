# Control Gator through conversation

Ask Gator to inspect or change its own settings. It uses the same stores and
services as the desktop screens, with tool results confirming actual changes.

| Example request | Behavior |
| --- | --- |
| “Which model are you using? Switch to …” | Inspects configured choices and selects an existing provider/model for subsequent messages. API keys and new local downloads use Models. |
| “Use local retrieval” or “use Gemini for saved recall.” | Reviews the retrieval provider separately from chat. Cloud retrieval explicitly reviews uploading eligible saved text and search queries. Local retrieval does not install a model automatically. |
| “How much have I used today?” | Reads current requests, tokens, limits, and activity counts. Incomplete provider cost data is not presented as a complete bill. |
| “Limit me to 100 requests a day.” | Shows the exact limits for review, then applies them after the reply. Zero means uncapped. |
| “Keep completed history for 30 days.” | Reviews the retention policy and saves it after the current run releases the profile. Saving retention does not run cleanup. |
| “Export a backup to /absolute/path/backup.tar.gz.” | Reviews the destination, then exports an archive after the run finishes. Existing files are not overwritten. |
| “Help me delete saved knowledge.” | Opens the existing file-count and scope review. Nothing is deleted until that screen is confirmed. Restore and source relinking likewise open their existing forms. |
| “Start background checks” or “enable start at login.” | Reviews the background process or per-user service change. Existing service ownership and configuration checks still apply. |
| “Use Reports for my next messages.” | Selects an already configured connection for this conversation's future messages. It does not sign in, change permissions, or add access to the running turn. |
| “What outputs did you create? Save that run's files to …” | Inspects verified retained outputs and reviews copying them to the requested folder. Existing differing files are not replaced. |
| “Rebind command search to Cmd+P.” | Saves the binding, updates the UI and help, and activates it immediately. Conflicts, typing keys, and reserved OS shortcuts are rejected. |
| “Reset the command search shortcut to default.” | Restores that action's default binding. |
| “Read new replies aloud” or “use Gemini for dictation.” | Updates voice preferences. Chat cannot activate the microphone. |

Connection selections persist separately from sealed historical run settings.
Selections already captured by queued requests retain their original scope.
App-control tools are available only to the foreground conversation, not to
background checks or delegated specialists. Questions alone do not authorize
changes, and source files or saved notes cannot authorize app mutations.

Pausing requests or exhausting a shared limit also stops new model-powered
chat and dictation requests. Use **Settings → Usage & limits** to resume or
raise limits when Gator cannot make another model request.

## Dictation and spoken replies

1. Open the microphone button beside the message box. Its default shortcut is
   **Cmd+Shift+V** on macOS and **Ctrl+Shift+V** elsewhere.
2. Check the displayed dictation provider and language. Gemini or OpenAI needs
   an API key configured through **Settings → Models**. A supported chat provider
   is reused unless a separate voice provider is selected.
3. Choose **Start recording**, grant the OS microphone permission, speak, then
   choose **Stop & transcribe**. Clips stop at two minutes and are limited to
   8 MiB. Audio is sent only to the selected provider for transcription.
4. Review the text added to your existing draft, then send it normally. Cancel,
   navigation, permission denial, and transcription failures preserve the draft.

Recording replaces the voice menu with a compact pill beside the model picker.
Its bars respond to the microphone locally. The square finishes recording;
the × button or Escape cancels it. Opening the microphone and transcribing use
short inline labels with a cancel button. Provider and language settings stay
in the voice menu. The elapsed timer and waveform motion are reduced in compact
windows and when reduced motion is enabled, respectively.

The speaker button enables optional playback of new replies using a locally
installed system voice. It also stops current playback. Existing conversation
history is not replayed when playback is enabled. Code blocks are skipped and
spoken text is bounded to 12,000 characters. Recording stops playback to avoid
capturing Gator's own voice.

Audio bytes are transient and are not saved in transcripts, logs, or artifacts.
The reviewed transcript becomes an ordinary draft; sending it follows the
normal conversation and retention rules. Voice provider calls count in shared
usage under **Voice dictation**; unreported token usage retains an estimate.
Changing the conversation or voice route during capture prevents that recording
from being added or uploaded under a different selection.

The implementation uses [OpenAI file transcription](https://developers.openai.com/api/docs/guides/speech-to-text)
and [Gemini inline media input](https://ai.google.dev/api/generate-content#Part).
Provider model availability, billing, and retention are governed by the selected
provider; Gator's local audio lifecycle does not establish the provider's policy.

## Files in the conversation

Open an attached or generated file card inside chat. Preview/Edit exposes only
the controls supported by that format. Save creates a private working version;
Ask Gator adds its saved version and selected text/cell to a reviewable message.
Neither action sends that message automatically. File edit drafts are separate
from composer drafts and recover on reopening the file. Export uses a native
save dialog and does not overwrite an original unless that destination is
explicitly chosen. See [file workflows](INLINE-FILES-RESEARCH.md) for native
Office limitations, version retention and tests.

## Development verification

```sh
make build
go test ./...
npm run check --prefix desktop
npm test --prefix desktop
npm run test:e2e --prefix desktop -- app-control.spec.ts
```

The integration fixtures use temporary profiles, local fake providers, generated
audio, and mocked OS speech/permission surfaces. They verify the actual Go
settings and accounting paths without using real accounts or a physical
microphone. OS microphone permission, real speech accuracy, and installed voice
availability still require checking on the intended host.

The implementation is divided into focused Git commits for rollback. Inspect
`git log --oneline` for the `feat(agent)`, `feat(app)`, `feat(config)`,
`feat(desktop)`, `feat(voice)`, and related verification commits. Revert dependent
changes in reverse order when rolling back a whole feature.
