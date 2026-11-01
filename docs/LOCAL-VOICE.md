# Local voice

Electron supports **Whisper · local, no API key** alongside Gemini and OpenAI dictation. Spoken replies use installed local system voices independently of the dictation provider. Whisper is not preinstalled by macOS or Linux.

## Setup

1. Install the optional [whisper.cpp runtime](https://github.com/ggml-org/whisper.cpp#quick-start) using its supported installation instructions. On macOS the Homebrew formula is `brew install whisper.cpp`. Gator does not install executable code or change the system toolchain. `whisper-cli` must be on the launcher’s PATH; a host administrator can instead set `GATOR_WHISPER_BINARY` to its executable. Packaged apps may have a different PATH from an interactive shell.
2. Open the microphone’s **Voice** menu, choose **Whisper · local, no API key**, and select Tiny, Base (default), or Small.
3. Click **Download local model** once. This explicitly fetches public multilingual model weights from Hugging Face. Tiny is approximately 74 MiB, Base 141 MiB, Small 465 MiB. A larger model uses more memory and may take longer. Downloading never starts recording. Cancel leaves any previous complete model intact.
4. Click **Start recording**, then **Stop & transcribe**. Review the text appended to the existing draft before sending. Escape cancels. Recordings are limited to two minutes.
5. Optionally enable spoken replies and select an installed **Local system voice**. Gator excludes network voices. Availability depends on OS speech services and installed voice packages; Linux distributions do not all ship a voice. Gator does not download or configure OS voices.

A missing runtime, missing model, unsupported platform, or local transcription error is explicit. There is no automatic cloud fallback. Selecting Gemini or OpenAI separately uses the configured API provider and its existing usage limits.

## Implementation and privacy

`internal/speech/local_models.go` owns a finite pinned model catalog. Downloads use immutable upstream revision `5359861c739e955e79d9a303bcbc70fb988958b1`, exact byte counts and SHA-256 checksums. Browser requests contain a catalog ID, never a URL, executable or destination path. HTTPS redirects are restricted to Hugging Face domains, without provider credentials. Complete verified files are atomically published under the private state directory’s `gator/speech/models/`. A per-profile cross-process lock prevents replacement during inference. Interrupted download fragments are removed by the next explicit download under that lock. There is no startup download or model daemon.

`desktop/src/localAudio.ts` converts browser audio in memory to bounded mono 16 kHz signed 16-bit PCM WAV using an offline Web Audio decoder. It does not play the recording. `internal/speech/local.go` validates the WAV, verifies model integrity, and sends audio on stdin to the host-installed `whisper-cli`, with fixed argv and no shell. Transcript stdout is limited to 32 KiB, diagnostics to 64 KiB, execution to 180 seconds. Local inference is CPU-only initially, serialized across helpers, cancellable, and inherits no API credentials. Temporary working directories contain no recording or transcript files and are removed on normal completion/cancellation. Forced OS termination can leave an empty working directory. The official tested runtime writes no transcript export with these arguments. A user-supplied executable remains trusted host code, not a sandboxed model.

Voice-route identity incorporates preferences and runtime/model file metadata. Existing conversation-bound microphone grants reject a changed route or conversation. Local inference bypasses the commercial API token ledger; it still uses local CPU, memory and power. It does not consume model-provider tokens. Recordings and transcripts are excluded from ordinary diagnostics. Transcripts become draft text only; sending the draft to a chat provider remains a separate human action.

## Platforms and provenance

The local inference integration supports macOS and Linux without CGO, ffmpeg, Python, or new Go/npm dependencies. Windows local inference currently reports unsupported; existing Windows preview and API dictation remain separate. This does not certify native Linux/WSL microphone or OS speech availability.

The API integration is Gator code; no Whisper application server or runtime source is vendored. Model files are downloaded only on request. [OpenAI Whisper](https://github.com/openai/whisper) and [whisper.cpp](https://github.com/ggml-org/whisper.cpp) use MIT licenses. Relevant license texts and provenance are retained in [third_party/whisper](../third_party/whisper/README.md); this does not change Gator’s licensing. The desktop license collector includes these notices in packaged `licenses/DEPENDENCIES.txt`.

## Verification

Deterministic Go tests cover pinned downloads, integrity, size limits, corrupt/linked storage, failed replacement, cancellation, concurrency, WAV validation, native output limits, credential isolation and sanitized failures. Electron tests use a synthetic audio stream to exercise actual MediaRecorder/Web Audio conversion and a clearly identified IPC fixture for download/transcription UI states. They do not claim microphone or recognition acceptance.

Real native acceptance on macOS Intel used a separately built, uninstalled whisper.cpp **v1.9.5**, the pinned Tiny model downloaded through Gator, and upstream public `samples/jfk.wav`. CPU transcription recognized the expected speech in 5.42 seconds, with no API calls. The native Go voice-helper setup/download/route/transcription test also passed (9.47 seconds) without API keys while commercial usage was paused, leaving zero voice charges. No physical microphone or privacy permission was changed. Native Linux/WSL/Windows microphone acceptance, signed installer permissions and subjective multilingual recognition quality remain unverified.

```sh
go test ./internal/speech ./internal/config ./cmd/gator
go test -race ./internal/speech ./internal/config
npm run check --prefix desktop
npm test --prefix desktop
GATOR_E2E_BACKGROUND=1 npm run test:e2e --prefix desktop -- app-control.spec.ts
```

Explicit native test (downloads 77.7 MB to a disposable profile; never uses the microphone):

```sh
GATOR_NATIVE_WHISPER_TEST=1 \
GATOR_WHISPER_BINARY=/absolute/path/to/whisper-cli \
GATOR_NATIVE_WHISPER_WAV=/absolute/path/to/whisper.cpp/samples/jfk.wav \
go test ./internal/speech -run '^TestNativeLocalWhisper$' -v -count=1
```

Manual acceptance: use Voice settings to download the selected model, cancel one download, record harmless speech, confirm draft preservation and playback voice selection, cancel recording/transcription, then test with networking disabled after setup. Verify macOS microphone permissions for the actual installed launcher. Do not infer physical microphone or signed installer acceptance from the synthetic UI test.

Final regression record: all 72 Go package suites and vet passed; speech/config race tests passed, as did the final seven-package coding/voice race run and focused rerun. Desktop TypeScript/build and Node tests passed. Both local/API voice journeys passed in development Electron and the unsigned packaged macOS/x64 app; packaged startup without a standalone Node or Gator CLI also passed. ASAR inspection confirmed optional Whisper licenses. Linux/amd64 and Windows/amd64 cross-builds succeeded with CGO disabled; no native runtime acceptance is implied. The full Electron run’s separate animation-test timing failure was corrected and passed a focused rerun; exact counts and exclusions are retained in [shared coding verification](SHARED-CODING-CAPABILITIES.md#final-automated-verification--2026-11-01).
