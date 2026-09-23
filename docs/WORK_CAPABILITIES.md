# Local Work capability matrix

Gator Work is local-first. A Work run writes only to its staged output until a
separate review/publish boundary is used. It is not a hosted workstation or a
cloud continuation product.

| Capability | Local Work v1 behavior | Boundary |
| --- | --- | --- |
| DOCX, XLSX, PDF | Semantic writers and deterministic artifact validation | Existing staged-output contract |
| PPTX | Create decks with text, notes, and fade/push/wipe transitions; inspect and apply bounded text/title/body/note/reorder/add/delete edits to source decks | Untouched OOXML parts are copied through. VBA, embedded objects, media, and unsupported animation data are never read, executed, authored, or regenerated. |
| Controlled browser | `gator browser start` or `attach`, followed by a selected session and `gator work --browser-session SESSION` | Only selected tabs and approved origins are visible. Every mutation has a fresh approval. A `gator-local` model cannot receive this capability. |
| macOS desktop | `gator desktop start --app BUNDLE_ID`, then `gator work --provider openai --desktop-session SESSION` | The foreground app must match the session allowlist before every action. System Settings, Terminal, iTerm, Keychain, Finder, clipboard shortcuts, file dialogs, modifiers, and secure fields are blocked. Every activation, click, keypress, and typed value has a fresh approval. |
| OpenAI computer use | OpenAI Responses structured `computer` calls drive the macOS desktop session | Only an explicit `openai` Work invocation with a desktop session enables it. The local model and non-OpenAI providers do not receive desktop/CUA tools. Screen captures are window-only, transient, and stripped from retained Work replay. `--retain-provider-state` is explicit per desktop session. |
| Scheduling | `gator job launchagent install` installs a per-user macOS LaunchAgent for the local job supervisor | Runs while the Mac is on and the user is signed in. It is not cloud/laptop-off scheduling. Scheduled jobs have no desktop/CUA session fields and therefore cannot automate apps. |
| Cloud/shared organization work | Not implemented | No hosted execution, cross-device continuation, shared workspace, admin plane, or cloud audit service. |

## PPTX editing guarantees

The presentation writer never uses PowerPoint as an automation dependency. It
validates bounded archive structure before editing and keeps source files
immutable: an edited deck is emitted as a new staged `.pptx` artifact. It
preserves unmodified package-part bytes semantically, but no ZIP rewriter can
promise byte-for-byte identity for container metadata. PowerPoint visual review
is optional and outside the structural writer; use it to review an output that
contains application-specific content not represented in Gator's typed edit
surface.

New deck media import and arbitrary animation authoring are intentionally not
part of v1. Existing media and animation parts survive edits unchanged. This
keeps the writer from flattening a deck simply because it encountered an effect
or an embedded object it does not understand.

## Practical start

```sh
gator browser install
gator browser start --visual-capture
# Select its tab and approve the needed origin, then:
gator work --browser-session BROWSER_SESSION --artifact brief.pptx "Create a product brief deck"

gator desktop start --app com.microsoft.Powerpoint --retain-provider-state
gator work --provider openai --desktop-session DESKTOP_SESSION --artifact review.md "Review the approved presentation window"
```

The desktop command requires macOS Accessibility permission. Screenshot access
may additionally require the operating system's Screen Recording permission.
Gator fails closed if either required operation is unavailable.
