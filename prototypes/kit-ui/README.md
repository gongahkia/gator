# Gator terminal UI preview

An isolated visual fixture built with `@claude-code-kit/ui` and its terminal renderer. It does not call a model or read Gator data. The live frontend is now in [`../../ui/`](../../ui/); keep this fixture for comparing layouts without touching real state.

From this directory:

```sh
npm ci
npm run preview
```

For a populated conversation screen, run `npm run preview -- --scene=brief`. Compare either scene with Claude Code in a second terminal pane of the same size.

When comparing, look at the home screen's density, the `/` command picker, how a finished answer reads, and the bottom input and status area at both narrow and wide terminal sizes.

The `/` menu contains Gator commands. Try `/goals`, `/inbox`, `/memory`, `/model`, `/new`, and `/exit`. Normal text receives a preview-only response. All goal, inbox, and brief content is synthetic.

The kit's REPL hardcodes cyan in its prompt and command suggestions. This preview composes the kit's message list, input, dividers, and status line with a Gator-green command picker and search prompt. Type `/`, use ↑/↓, and press Enter or Tab to run a preview command. No dependency fork is needed for this prototype.

