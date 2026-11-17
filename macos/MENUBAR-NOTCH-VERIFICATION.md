# Menu bar and notch interaction verification — 2026-11-17

This is a user-requested Tranche08 continuation, not Tranche09. Final local bundle: **`/tmp/Gator-Native-Tranche08-Enhanced-Final.app`**. Earlier acceptance, diagnostics, notch-preview and intermediate enhanced bundles are preserved. This report supplements the original closure evidence.

## Behavior and implementation

The original alligator menu icon remains. A discrete symbol and accessible status describe working tasks or chats needing inspection. State transitions use short opacity changes, without a continuously running attention animation.

On a display that reports a real camera gap and positive safe-area height, Gator presents a small black quick-access panel around the notch. The tested M3 display reports 1470×956 points, a 179-point camera gap and 32-point safe top. The collapsed panel is 243×38 points; the expanded panel is 420×246. Flat displays produce no notch panel. Geometry tests also cover negative display origins; physical external-display rearrangement was not tested.

Hover opens after 160ms; leaving closes after a 320ms grace period. Hover does not make the panel key. Clicking its header or using **Conversation → Show Notch Quick Access (⌘⇧G)** explicitly enables keyboard interaction. Dismiss starts with keyboard focus; Tab/Shift-Tab reach the chat rows and return; Escape restores the previous window. Moving to another key window dismisses the explicitly opened panel without activating another window. The More menu contains an application-only Notch Quick Access preference. No persistent OS accessibility settings were changed.

The expanded surface shows existing owner activity, attention and work counts. A row opens its original conversation, then its current exact action review when available, otherwise task inspection. Open Chat restores the composer. The panel offers no approval, send, replay or retry decision. Presentation uses the existing `ManagedSessionCoordinator`; it owns no runtime or durable authorization. Screen-parameter observers reposition it; shutdown removes observers and closes the panel.

Animations use an 180ms ease-out transition, disabled through SwiftUI's Reduce Motion environment. The black surface joins the physical camera area; standard SwiftUI text, symbols, buttons and focus indicators carry the controls. There is no decorative material stack.

[boring.notch](https://github.com/TheBoredTeam/boring.notch) informed the nonactivating panel and hover expansion interaction. Its inspected source is GPL-3.0; no source file or dependency was imported. The Gator panel and owner routing are original implementation. Reference revision and installation metadata are in `Evidence/All-Providers/headless/installation-metadata.json`.

## Native observations

Observed on the actual final bundle, in a disposable profile with a loopback provider and no account credentials:

| Check | Result | Evidence / limits |
| --- | --- | --- |
| Real notch detection | PASS | Actual NSScreen safe area and auxiliary areas; geometry recorded in `native-interactions.json`. |
| Explicit keyboard entry | PASS | ⌘⇧G → Dismiss quick access; Tab → New chat, Idle; Shift-Tab → Dismiss. |
| Escape / draft restoration | PASS | Returns to Message Gator with `An unsent draft for notch focus acceptance` intact. |
| Hover preserves composer focus | PASS | Focus remains Message Gator while panel expands. |
| Close full window → notch Open Chat | PASS | Same draft and same Go child PID57058 before/after. |
| Presentation does not add an owner | PASS | One child before/after the preceding controlled presentation journey. Conversation navigation and task dispatch can legitimately add owners; they are separate operations. |
| Working activity | PASS, intermediate binary | Actual controlled working state in `final-working.png`; despite its filename this predates the final keyboard-focus fix. |
| Exact approval route through notch | NOT TESTED end to end | Code routes only after confirmed owner selection. The current fixture produced a terminal retained result rather than a pending approval; it does not establish this journey. Existing exact approvals retain the earlier closure evidence. |
| Physical VoiceOver, Full Keyboard Access and preference variants | PARTIAL | Operator checklist still required; focus/AX metadata does not establish spoken behavior or OS preference variants. |
| Flat-display fallback / other display layouts | PASS headlessly / NOT TESTED physically | Geometry rejection tests; only the actual built-in notched display was exercised. |

A first native check found that explicit entry left focus on the panel rather than a control. The final implementation explicitly focuses Dismiss and marks the buttons keyboard focusable. The final native traversal above follows this change. Original and final Swift logs are retained separately.

Some compound automation attempts missed menus or controls. A temporary Unicode input helper also inherited modifier flags; those mixed batches are excluded from draft/owner assertions. Its corrected helper sets empty modifier flags explicitly. The small, settled final journey above supplies the accepted evidence.

## Screenshot index

Paths are relative to `Evidence/Notch-Quick-Access/`.

| Image | Provenance and use |
| --- | --- |
| `final-expanded-screen-region.png` | Actual final bundle, tightly bounded OS screen capture; primary expanded visual reference. Includes the physical camera-area context. |
| `final-full-recovery-screen.png`, `final-compact-recovery-screen.png` | Actual final bundle, controlled retained recovery inspection in full and compact layouts; no approval or replay decision. |
| `final-full-recovery-window.png` | Window-only supporting recovery capture; screen-region versions are preferred for visual claims. |
| `final-collapsed.png` | Final bundle, window-only collapsed surface. |
| `final-keyboard-expanded.png`, `final-idle-settled.png`, `final-idle-expanded.png` | Window-only captures omit some compositor layers. Excluded from typography/completeness claims; AX/focus observations remain separately recorded. |
| `final-working.png` | Intermediate `/tmp/Gator-Native-Tranche08-Enhanced.app`; real working fixture, before final focus change. |
| `preview-expanded-settled.png` | Preview bundle, before explicit keyboard-focus fix; supporting layout reference only. |
| `preview-expanded.png` | Captured during transition; excluded. |
| `preview-collapsed.png` | Preview bundle; supporting camera-gap layout only. |

The existing [Tranche08 screenshot index](Evidence/Tranche08-Closure/continuation/SCREENSHOT-INDEX.md) remains the evidence for conversation, attachments, long transcripts, exact approvals, plans, recovery, themes and resizing. Those are historical generations, not new final-binary captures.

## Verification and packaging

- `go build -o /tmp/gator-native-runtime ./cmd/gator`: PASS.
- Relevant complete Go regression: all15 packages PASS; command is recorded in the provider supplement and logs.
- Swift build PASS. First complete pass132 PASS /39.120s. After the native focus fix, final complete pass **132 PASS /37.262s**.
- Host16 PASS; recovery integration9 PASS /208 assertions; selected TUI2 PASS /45 assertions and1 existing platform skip.
- Two new geometry tests cover real-gap requirements, malformed/flat screens, sizing and translated origins. Existing ownership, draft, approval and recovery tests remain enabled.
- Build script, plist, actual native/Go Mach-O arm64 files, Go module metadata, native-to-build hash match and literal credential scan: PASS. Details in `bundle-verification.json`.
- Final bundle has the real Go runtime and existing original PNG/ICNS branding. No credential was embedded. No signing/notarization claim.

Native code does not change the transcript projection optimization. No new rendering/energy benchmark was conducted for this panel; the previous bounded performance measurements remain historical. Sustained energy profiling and physical accessibility acceptance remain outstanding.

Additional final checks: custom/keyless/managed-local rejection through metering and existing private CLI trace persistence both PASS (`gator-enhanced-custom-trace-check.log`). A credential-free probe of the actual bundled Go executable and controlled fixture cleanup are recorded in `bundled-runtime-probe.json` and `fixture-summary.json`.
