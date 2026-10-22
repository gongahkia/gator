# Desktop releases and reviewed updates

## Current state

The repository has a signed-release configuration, native verification gates,
architecture-specific update feeds and a reviewed update experience. Apple and
Windows signing accounts/certificates are **not configured yet**. No signed,
notarized or published release has been produced by this change.

`npm run pack --prefix desktop` and `make dist` keep producing development
builds without requiring signing credentials. Tagged production builds require
complete signing configuration and fail before packaging if it is absent.

## What users see

Open **Settings > About & updates**, or the application menu's equivalent.
Nothing checks for an update or downloads one at launch. In a supported signed
installation the user explicitly checks, downloads, then reviews a restart.
Ordinary quit never installs a downloaded update.

| Installation | Update behavior |
| --- | --- |
| Signed macOS app | `electron-updater`, signed DMG/ZIP distribution |
| Signed Windows NSIS installation | `electron-updater`, publisher-verified NSIS installer |
| Windows portable ZIP | Manual update; detected by absence of the NSIS uninstaller |
| Linux AppImage/DEB | Package manager or manual release installation |
| Local/CI preview | Clearly marked manual development update |

Each architecture has its own channel (`stable-x64`, `stable-arm64`). Windows
metadata uses `stable-ARCH.yml`; macOS uses `stable-ARCH-mac.yml`. Keeping these
separate avoids merging incompatible x64/ARM64 metadata. Prereleases, downgrades
and web installers are disabled. The renderer cannot select a feed or executable.

## Preserving work

The host checks for active work, queued messages, approvals, external handoffs
and editable forms. The renderer also protects queue edits and composer text
that has not reached the local draft store, including disabled retention.

After the native restart review, the Go application checks readiness inside
its ordered message loop. It acknowledges an idle session before exiting;
a stale host snapshot cannot cause an active run to be forcibly stopped.
This update path has no force-kill timeout.

A private helper then takes the existing exclusive profile-data lease and
refuses active writers or a live/invalid supervisor lock. Disable start at login
and stop background checks through Recovery before updating. On macOS the login
service uses KeepAlive and would otherwise restart the supervisor. Saved check
definitions remain; re-enable start at login after updating. The updater does
not automatically restart checks. This avoids
claiming an unimplemented supervisor stop/drain/restart transaction.

The preparation lease is released after the idle session has exited and before
the installer replaces application files. It is not an atomic lock across the
whole OS installation. Other installations/profiles are outside this guard;
close them before updating a shared installation. Neither the helper nor the
installer migrates or deletes saved profiles. Future incompatible data migrations
need their own backup/compatibility protocol.

Draft writes finish before preparation. A failed preparation reconnects only if
the old session actually exited, and never replays a request. Installer errors
reported while the host is alive attempt the same recovery. After the OS takes
over installation, native failure/relaunch handling still needs platform tests.
Admitted messages and captured queue settings are retained per conversation.
After restart, queued messages remain paused for explicit selection; a run whose
completion was not recorded is shown as interrupted. It may already have made
external changes, so it is never replayed automatically. Retrying creates a new
attempt. This restores the request, not the prior model's execution checkpoint.

Relative profile overrides retain their existing platform interpretation:
workspace-relative on Unix, host-resolved in the Windows preview. The guard uses
the same launch workspace and environment as the foreground runtime. A packaged
regression test seeds existing data and verifies that the app still opens it.

## One-time signing setup

Use the GitHub `desktop-release` environment. Required reviewers are an external
repository setting; committing the workflow does not establish that protection.
Credentials below belong in GitHub secrets, not source, chat or local Git config.
The Windows signing identity is separate from Azure DevOps Git authentication.

### macOS

Obtain a Developer ID Application identity through an Apple Developer account.
Export its certificate/private key for the CI signing keychain, and configure:

| GitHub secret | Meaning |
| --- | --- |
| `MAC_CSC_LINK` | Base64 `.p12` signing certificate/private key |
| `MAC_CSC_KEY_PASSWORD` | Export password |
| `APPLE_ID` | Account used for notarization |
| `APPLE_APP_SPECIFIC_PASSWORD` | App-specific notarization password |

| GitHub variable | Meaning |
| --- | --- |
| `MAC_SIGNING_IDENTITY` | Exact `Developer ID Application: …` identity |
| `APPLE_TEAM_ID` | Signing/notarization team |

Release builds enable hardened runtime, JIT entitlements for Electron and
notarization. The private Go runtime is explicitly included in signing. Native
verification checks the app and runtime signatures against the configured team,
validates the stapled ticket and runs Gatekeeper assessment before upload.

### Windows

Choose a signing provider appropriate to your account and certificate. The
configuration supports Azure Trusted Signing or an eligible CI certificate.
Account eligibility, identity validation and provider access still require setup.
Do not assume a hardware-backed certificate can be exported to a PFX.

For **Azure Trusted Signing**, set `WINDOWS_SIGNING_PROVIDER=azure` and these
environment values:

| GitHub variable | Meaning |
| --- | --- |
| `WINDOWS_PUBLISHER` | Exact certificate publisher name, used to verify updates |
| `WINDOWS_SIGNING_ENDPOINT` | Regional signing endpoint |
| `WINDOWS_SIGNING_ACCOUNT` | Signing account name |
| `WINDOWS_SIGNING_PROFILE` | Validated certificate profile name |

Secrets: `SIGNING_AZURE_TENANT_ID`, `SIGNING_AZURE_CLIENT_ID`,
`SIGNING_AZURE_CLIENT_SECRET` for the signing service principal. These are consumed
only in tagged Windows CI builds; no ADO credentials or global Git helpers change.

For a provider supporting a **CI-importable certificate**, leave
`WINDOWS_SIGNING_PROVIDER` unset or use `certificate`, set `WINDOWS_PUBLISHER`, and
configure `WINDOWS_CSC_LINK` and `WINDOWS_CSC_KEY_PASSWORD` secrets. A provider
requiring different hardware/API signing needs an adapter before releasing.

The app, extra-resource Go runtime and NSIS installer are signed. Native
verification uses `Get-AuthenticodeSignature` and checks the publisher. Signing
does not establish SmartScreen reputation; installation still needs a real test.

## Release procedure

1. Run source checks and native platform tests. Ensure version and saved-data
   compatibility have been reviewed. Choose a stable `vMAJOR.MINOR.PATCH` tag.
2. The tag workflow checks credentials, builds every configured platform/arch,
   verifies signatures/notarization where applicable, and validates metadata
   SHA-512 hashes against the actual artifacts. x64 Windows also runs packaged
   UI tests; cross-packaging ARM64 does not prove native ARM64 runtime behavior.
3. All jobs must succeed before a **draft GitHub release** is created with
   installers, blockmaps, per-architecture feeds and SHA-256 checksums. Build
   configuration/debug files are excluded from uploaded assets.
4. Download that draft on clean native machines. Test install, launch, sign-in,
   existing-profile preservation, background-service handling and uninstall.
   Test an update between two signed versions, not merely a fresh installation.
5. Review the draft and publish it explicitly. Drafts are not visible to the
   normal public updater. A rollback of a published update should be a newer
   corrective version; automatic downgrades are disabled.

No tag, push, signing-account change or release publication is performed as
part of the local implementation work.

## Local verification, 22 October 2026

| Check | Observed result |
| --- | --- |
| Go application model, command services and new update guards | Pass |
| Repository-wide Go tests | Every package passed across the initial run and serial reruns. Five initial binaries stalled at macOS `_dyld_start`, before Go initialization; those isolated processes were stopped and their packages all passed serially. No test assertion was removed. |
| `go vet ./...` | Pass |
| Windows x64 application/update tests | Compile-only with an execution stub; not native execution |
| Windows x64 vet / Windows ARM64 runtime build | Pass |
| Desktop TypeScript | Pass |
| Host/controller/release tests | 27 pass; one native Windows handoff test skipped on macOS |
| Unsigned macOS application packaging | Pass |
| Packaged desktop suite | 17/17 pass in the final complete run, including relative-profile preservation, About & updates, real bundled updater loading, draft preservation and no standalone CLI/Node requirement |
| Real macOS ZIP update manifest | `stable-x64-mac.yml` generated; version, architecture and artifact SHA-512 verified in an isolated unsigned preview; temporary bundle removed |
| Workflow YAML | Both changed workflows parse |
| Signing, notarization, native Windows installer/update, ARM64 hardware, remote CI and release publication | Not executed; require account setup and/or native infrastructure |

The first expanded About test used a dynamic import inside Playwright's remote
evaluation, which lacks a dynamic-import callback. It was replaced with the
native module loader in the test. The same assertions passed, including loading
the actual packaged updater; production import behavior was not changed for that
test issue. The complete packaged suite was then rerun successfully.

Interface review: the update dialog uses the app's existing React/CSS tokens,
modal focus handling, Lucide icons and reduced-motion behavior. Manual-build
presentation, dismissal/focus return and unsent-message preservation were
inspected in the packaged app. Download progress uses tabular numbers and actual
updater events. It adds no automatic startup checks or persistent chat banner.
Native signed download/install screens remain a release verification gate.

## References

- [electron-builder 26 update behavior](https://www.electron.build/v26/docs/features/auto-update/)
- [electron-builder signing configuration](https://www.electron.build/v26/docs/features/code-signing/)
- [Electron code signing](https://www.electronjs.org/docs/latest/tutorial/code-signing)

The installed electron-builder/electron-updater sources are also inspected for
exact options and event behavior; online guides may describe newer versions.
