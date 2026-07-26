# Loading dialogs

Gator shows a non-focusing floating loading dialog while it checks local providers and starts a run. It is local UI only; it sends no data and does not add a Rust, Ruby, or network dependency.

```lua
require("gator").setup({
  ui = {
    loading = {
      enabled = true,
      spinner = "rattles.braille.dots",
      interval_ms = 0,
    },
  },
})
```

`enabled = false` suppresses dialogs. `interval_ms = 0` preserves the preset's upstream cadence; a value of at least `16` overrides it. Reduced or disabled `ui.motion` displays the first frame without animation.

Get the exact fully-qualified names in Neovim:

```vim
:lua =require("gator.ui.loading").presets()
```

There are 169 presets: `rattles.arrows.*`, `rattles.ascii.*`, `rattles.braille.*`, `rattles.emoji.*`, `whirly.*`, and `whirly.cli.*`. Fully-qualified names deliberately preserve duplicate upstream names such as `dots`. The Whirly procedural presets are `whirly.random_dots`, `whirly.mahjong`, `whirly.domino`, and `whirly.vertical_domino`.

The data is vendored from [Rattles](https://github.com/vyfor/rattles) and [Whirly](https://github.com/janlelis/whirly), including Whirly's bundled cli-spinners data. Gator implements the upstream linear, swing, and random frame behavior natively in Lua. See [third-party notices](../THIRD_PARTY_NOTICES.md).
