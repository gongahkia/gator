# Gator terminal UI preview

An isolated visual prototype built with `@claude-code-kit/ui` and its terminal renderer. It does not call a model, read Gator data, or replace the Go CLI. The goal is to judge the terminal layout before committing to a frontend migration.

From this directory:

```sh
npm ci
npm run preview
```

For a populated conversation screen, run `npm run preview -- --scene=brief`. Compare either scene with Claude Code in a second terminal pane of the same size.

When comparing, look at the home screen's density, the `/` command picker, how a finished answer reads, and the bottom input and status area at both narrow and wide terminal sizes. This preview is a decision aid, not a frontend migration.

The `/` menu contains Gator commands. Try `/goals`, `/inbox`, `/memory`, `/model`, `/new`, and `/exit`. Normal text receives a preview-only response. All goal, inbox, and brief content is synthetic.

The REPL component currently renders command suggestions with its built-in cyan selection color. A full green theme would require a theme option upstream or a small fork. This preview deliberately leaves that component unchanged so its actual appearance is visible.

