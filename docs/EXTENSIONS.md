# Extensions

Gator extensions are explicitly configured trusted Lua modules. They are not sandboxed and are not versioned APIs: update an extension with Gator when its interfaces change.

```lua
require("gator").setup({
  extensions = { modules = { "my_gator_extension" } },
  ui = {
    renderers = { provider_picker = "my-picker" },
    run_graph = { columns = { "id", "provider", "state", "my-column" } },
  },
})
```

The module returns a setup function, or `{ setup = function(api) ... end }`. Setup may return a cleanup function. Gator disables an extension after its callback fails, removes its registrations, reports it in `:GatorHealth`, and keeps the core run alive.

```lua
return function(gator)
  gator.events.on("run.finished", function(event)
    vim.notify(event.provider .. " " .. event.state)
  end)

  gator.ui.column({
    id = "my-column",
    label = "Branch",
    render = function(model)
      return vim.fn.fnamemodify(model.run.workspace.root, ":t")
    end,
  })
end
```

Events are `run.created`, `run.started`, `run.state_changed`, `run.finished`, `approval.requested`, `approval.resolved`, `context.prepared`, `context.delivered`, `handoff.prepared`, `handoff.reviewed`, `handoff.delivered`, and `status.changed`. Gator also emits corresponding `User` autocmds: `GatorRunCreated`, `GatorRunStarted`, `GatorRunStateChanged`, `GatorRunFinished`, `GatorApprovalRequested`, `GatorApprovalResolved`, `GatorContextPrepared`, `GatorContextDelivered`, `GatorHandoffPrepared`, `GatorHandoffReviewed`, `GatorHandoffDelivered`, and `GatorStatusChanged`. Payloads are redacted Gator metadata; they do not include raw prompts, diffs, terminal output, or credentials.

Outside an extension, use `require("gator").on(event, callback)` for a local subscription. It returns an unsubscribe function. Use `require("gator").statusline({ fields = { "active", "provider", "state" } })` from an existing statusline configuration; Gator never changes the statusline itself.

## Providers

Terminal adapters register `name`, `kind = "terminal"`, `probe`, `start`, and optional `resume`. `probe` returns `{ available = boolean, version = optional_string }`; `start`/`resume` return `{ session = { id = string }, command = { argv... } }`. They open a native Neovim terminal and remain provider-owned for authentication, permissions, sandboxing, network, and MCP behavior.

ACP adapters register `name`, `kind = "acp"`, `argv`, and `probe`. Gator uses the configured command through its ACP transport; custom callbacks do not replace the ACP session protocol.

## UI and context

`gator.ui.renderer({ id, slot, render })` may replace `provider_picker`, `run_graph`, `context_preflight`, `handoff_review`, or `dashboard` when selected by `ui.renderers`. The renderer receives a copied redacted model and explicit `actions`; return `false` to request Gator's native fallback.

`gator.ui.column({ id, label, render })` adds a graph column selected through `ui.run_graph.columns`. Built-in column IDs are `id`, `provider`, `role`, `state`, `context`, `resources`, `budget`, and `trust`.

`gator.context.collector`, `redactor`, and `handoff_formatter` register named callbacks. Collectors add context artifacts, redactors run before delivery and persistence, formatters add reviewed handoff sections, and Gator runs its built-in redaction again before delivery. No extension can make a terminal provider Gator-sandboxed or credential-verified.
