# Gator terminal product: retired

Electron is the sole primary application as of 1 November 2026. The OpenTUI
renderer, Bun launcher, terminal distribution and terminal Make targets have
been removed after mapping their backend-connected capabilities and porting
missing interactions into Electron. See [the inventory and verification
record](ELECTRON-CONSOLIDATION.md).

From a checkout, use `make run` to build and launch Electron, `make build` to
build it, `make package` for a local app bundle, and `make dist` for installers.
No Bun runtime is required. See [development](DEVELOPMENT.md).

Shared Go services, data formats, coding-agent protocols, background checks,
optional tmux/companion support and exact approvals remain. In-app slash
commands and command search operate inside Electron; the private Go runtime
retains explicit developer/maintenance handlers, not a public CLI product.
System Terminal can still open for an explicitly chosen external editor or
Go-reviewed sign-in/install step. These handoffs never start a Gator TUI.

Existing installations and user data are not uninstalled or deleted. Finish
work before changing an installation. If an existing background service points
to a retired terminal package, use Electron's Recovery → Repair start at login
and review the new executable/profile binding before removing the old package.
Do not remove an executable still used by an enabled service. No service repair
or automatic start is performed by this source change.

Historical migration and native test results are preserved in
[the superseded ledger](TERMINAL-MIGRATION-VERIFICATION.md); they are not evidence
of a current terminal release or of a newly tested Electron installer.
