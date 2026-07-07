# Demo Recording

The README demo is generated from a deterministic local fixture. It starts a temporary
OpenAI-compatible fake server, creates a small failing Go module, runs `paw`, and records the repair.
No model API key or network service is required.

Regenerate the cast and GIF from a clean checkout:

```sh
make build
asciinema rec --overwrite --cols 96 --rows 24 --idle-time-limit 1 \
  --command "bash docs/demo.sh ./bin/paw" docs/demo.cast
agg --cols 96 --rows 24 --theme github-dark --idle-time-limit 1 \
  --select 0.14..100% docs/demo.cast docs/demo.gif
```

Expected outputs:

- `docs/demo.cast`
- `docs/demo.gif`

The recording should stay under 60 seconds.
