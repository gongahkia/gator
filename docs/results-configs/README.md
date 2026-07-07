# Benchmark Result Configs

Every populated row in [docs/RESULTS.md](../RESULTS.md) must link to the exact config used for that
run.

Conventions:

- File name: `<run-id>.toml`.
- Include every non-secret `paw` setting needed to reproduce the run.
- Do not commit API keys or provider credentials.
- Record secret environment variable names as comments when needed.
- Keep the `run_id`, dataset id, model ids, and hardware string aligned with the results row.
- Use [example.toml](example.toml) as the starting point for new runs.
