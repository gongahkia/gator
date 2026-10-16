# Archived ACP adapter

Gator previously exposed a coding-oriented Agent Client Protocol adapter through
`gator agent acp` and `gator --mode acp`. Those entry points are retired. Gator's
product surface is the terminal conversation for persistent goals, knowledge,
and recurring work. Local automation can use the Work JSONL protocol through
`gator work-rpc`.

The former adapter's implementation and tests remain in `internal/acp` as
historical regression material. It is not launched by the Gator CLI, and its
Code-required `execute` mode is not a supported product path.
