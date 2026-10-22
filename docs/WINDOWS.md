# Windows desktop port

Status: **Windows preview implementation; native verification pending.**
Decision: 22 October 2026. The desktop app remains the user entry point.

## Target and scope

Windows 11 x64 is the initial native validation target. Windows ARM64 has a
matching runtime and packaging target, with native ARM64 testing still required.
There is no 32-bit Windows target. This port preserves the Go agent loop,
approval model, provider interfaces, and saved-data formats.

| Area | Windows implementation | Remaining boundary |
| --- | --- | --- |
| Chat, recent conversations, drafts, knowledge, goals, context, outputs and usage | Native Electron frontend with bundled Go runtime | Packaged Windows workflow tests must pass on a Windows runner |
| Runtime and installer | `gator-runtime.exe`, per-user NSIS installer and ZIP, x64/ARM64 resources, alligator ICO and app identity | Signing and native install/update/uninstall verification remain production-release gates |
| State and configuration | `%LOCALAPPDATA%\Gator\state` and `%LOCALAPPDATA%\Gator\config` | Overrides keep their existing precedence; this adds no device sync |
| Private files | Protected, inheritable Windows DACLs for the current user, SYSTEM and administrators | Requires an ACL-capable filesystem; Unix mode bits alone are never treated as Windows privacy evidence |
| Shared locks and active-run leases | `LockFileEx` / `UnlockFileEx`, including shared and nonblocking modes and kernel release on exit | Native contention/crash tests are included in CI |
| Restore/relink | Rooted Windows no-replacement profile publication, file flushes, native path validation | Unix directory `fsync` has no equivalent through these directory handles; cross-OS archive/path migration is not claimed |
| Background checks | Detached native supervisor; reviewed per-user `HKCU\Software\Microsoft\Windows\CurrentVersion\Run\GatorBackgroundChecks` entry | Requires an active signed-in session; Windows may delay or disable startup; Recovery checks live health separately |
| External authentication/install handoffs | System PowerShell opens a console; native argument quoting and explicit exit results | Native Windows handoff tests included; live account sign-in is not exercised automatically |
| Browser tools | Authenticated IPv4 loopback controller, private endpoint/token files, pinned Playwright driver | Optional external Node/npm and the reviewed browser runtime installation are required; real Chromium on Windows needs separate verification |
| Local Ollama | Native memory/disk checks, user model directory and official Windows installation help | Requires Ollama and adequate resources; no GPU compatibility/performance claim |
| Strict subprocess sandbox | Explicitly unavailable | Default strict policy fails closed; this port does not switch users to host access |
| Interactive agent terminal tools | Explicitly unavailable | Needs a Windows terminal implementation and suitable isolation |
| Computer control | Explicitly unavailable, as on Linux | Existing computer-control implementation is macOS only |
| Native system notifications | No Windows toast implementation | Follow-up results remain available in Inbox |

Subprocess features that require strict isolation include process tools, hooks,
extensions and applicable MCP/LSP integrations. A Windows-capable UI does not
make those isolation-dependent capabilities available. Existing deliberate
sandbox-off developer settings retain their meaning; they are never enabled
automatically and do not make the unsupported PTY tools available.

## Platform decisions

- Keep Unix file-lock behavior on macOS/Linux behind a common file-lock API.
  Windows locks reserve a byte past the lock file's content so mandatory byte
  locks cannot prevent reading metadata. Close leases before deleting them on
  Windows, while retaining shared-ledger serialization and the private-data gate.
- Validate lock-file handles against rooted `Lstat` results. Reject final
  symlinks and Windows device, drive-relative and alternate-stream paths.
- Initialize private Windows profile/config roots before opening saved data.
  The desktop supplies its own draft directory for the same native ACL
  bootstrap. Credential and capability reads check DACLs where required.
- Publish restored profiles relative to held directory handles with Windows
  replacement disabled. Keep file flush errors visible; adapt only directory
  flushing to the host API.
- Use a small native start-at-login command plus a private profile descriptor.
  This binds the executable, state and configuration without squeezing long
  profile paths into the Run value's command-line limit. Unknown or other-profile
  registrations are not replaced or removed. Recovery binds changes to a digest
  of the registration and profile descriptor. Re-enable after moving the app.
- Let the reviewed supervisor role initialize its selected profile before
  maintenance. Its worker processes retain the app's private routing convention.
- Keep background app-owned commands hidden on Windows. Reviewed interactive
  handoffs get their own console. Encode handoff data separately and pass native
  argv directly through `ProcessStartInfo`; never interpolate it into `cmd.exe`.
- Use loopback on Windows instead of Unix sockets. Every browser RPC retains
  its per-session random token and existing tab/origin permissions. Endpoint
  files accept only numeric `127.0.0.1` addresses and valid ports.
- Execute npm's JavaScript entry point through Node on Windows; do not ask
  `CreateProcess` to run `npm.cmd` or pass installer arguments through a shell.
- Check the bundled executable's file format and architecture before packaging.
  A runtime left over from a different target fails the build. Windows runtime
  builds use the GUI subsystem; the app continues to communicate through pipes.
- NSIS uninstallation stops the matching login supervisor. Full removal deletes
  only this installation's exact Run command; an update preserves it. Saved app
  data is retained. Interactive checks without a login registration should be
  stopped in Recovery before replacing/uninstalling a preview build.

## Develop and package on Windows

Use Go 1.25.13 and Node.js 22.12 or newer for development. A Windows build does
not require Make, Git Bash, WSL, Swift, a system Gator CLI or a C compiler.

```powershell
npm ci --prefix desktop
npm start --prefix desktop
```

From the repository root:

```powershell
npm run check --prefix desktop
npm test --prefix desktop
npm run pack --prefix desktop
npm run dist --prefix desktop
```

The unpacked x64 app is `desktop\release\win-unpacked\Gator.exe`.
To test the packaged application:

```powershell
$env:GATOR_DESKTOP_TEST_EXECUTABLE = (Resolve-Path 'desktop\release\win-unpacked\Gator.exe').Path
npm run test:e2e --prefix desktop
```

`GATOR_STATE_DIR` and `GATOR_CONFIG_DIR` still select a profile.
`XDG_STATE_HOME` / `XDG_CONFIG_HOME` retain their existing override precedence.
`GATOR_DESKTOP_USER_DATA` selects Electron preferences/drafts; those are separate
from the Go profile. Installed apps bundle Electron and Go. External Node is
needed only for optional tools that explicitly require it.

## Verification and promotion

The Windows CI job compiles every test package and vets Windows source. Its
native suite covers locks and process exits, Windows DACLs, shared usage,
profile selection, credential/note persistence, restore/relink/publication,
browser authentication, local resource detection, path rejection and default
sandbox refusal. Host tests exercise PowerShell native argv/exit handling.
Packaged Electron tests exercise the same 15 workflows as macOS, including
startup without a standalone CLI or external Node on PATH.

Release jobs build x64 and ARM64 NSIS/ZIP artifacts, verify matching runtime
architecture, and run the x64 packaged workflows before publication. The ARM64
artifact is cross-packaged on Windows x64; this is not native ARM64 execution.

### Local results (macOS host)

| Check | Observed result |
| --- | --- |
| `go test ./...` | Pass across the repository; focused auth/terminal/sandbox/usage checks also pass after the final platform changes |
| `go vet ./...` | Pass for the host and Windows x64 source |
| Pinned Go 1.25.13 Windows x64 / ARM64 builds | Both compile as PE GUI executables |
| Pinned Windows test compilation | Every Windows test package compiles; `/usr/bin/true` acts only as a cross-compilation execution stub, so no Windows test was executed locally |
| Linux ARM64 runtime build | Pass after the portability changes |
| TypeScript | Pass |
| Host tests | 11 pass on macOS; the native Windows PowerShell test is correctly skipped here |
| macOS app packaging | Pass; packaged host modules and runtime match their source build |
| macOS packaged workflows | First full run: 14 pass, one shutdown timeout after all UI assertions passed. The affected workflow passes against the final clean rebuild, including shutdown (25.4 seconds) |
| Windows x64 unpacked cross-package | Created on macOS with executable resource editing/signing disabled for this local check; runtime bytes, GUI subsystem, preload and packaged alligator ICO verified |
| Packaging mismatch guard | Accepts the matching x64 and ARM64 runtime formats; rejects x64 when asked to package ARM64 |
| Windows ICO | Valid 256px PNG-in-ICO container derived from the existing alligator asset |
| Workflow YAML and diff checks | Both changed workflow files parse; `git diff --check` passes |

An intermediate macOS bundle failed to launch; development Electron and its
bundled Go transport both started. A clean rebuild held against unchanged source
starts and passes the affected packaged workflow. No product assertion was
removed and no timeout was increased. Windows tests and native installer
operations have not been executed on this Mac; no installer or release was
published by this change.

Do not describe Windows as production-verified until native CI and manual
installer checks have completed. Signing, startup/logoff/reboot behavior,
native terminal-window closure, live OAuth, Chromium installation, native
ARM64 and cross-OS backup migration are separate verification boundaries.

## Implementation references

- [Microsoft: file-range locking](https://learn.microsoft.com/en-us/windows/win32/fileio/locking-and-unlocking-byte-ranges-in-files)
- [Microsoft: protected file DACLs](https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-setnamedsecurityinfow)
- [Microsoft: per-user Run registration and command-line limits](https://learn.microsoft.com/en-us/windows/win32/setupapi/run-and-runonce-registry-keys)
- [Microsoft: PowerShell process launching](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.management/start-process?view=powershell-5.1)
- [electron-builder: NSIS options and installer hooks](https://www.electron.build/v26/docs/nsis/)
- [Ollama: native Windows installation and model storage](https://docs.ollama.com/windows)
