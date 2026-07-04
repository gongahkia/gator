# paw Harbor adapter

Build the Linux binary before running Harbor:

```sh
make build-linux
test -x bin/paw-linux-amd64
```

Install Harbor and this adapter:

```sh
uv tool install harbor
uv pip install -e adapters/harbor
```

Required environment:

```sh
export PAW_HARBOR_BINARY=bin/paw-linux-amd64
export PAW_BRAIN_TRANSPORT=openai
export PAW_BRAIN_BASE_URL=https://api.z.ai/api/paas/v4
export PAW_BRAIN_API_KEY=...
export PAW_BRAIN_MODEL=glm-4.6
export PAW_DRONE_TRANSPORT=openai
export PAW_DRONE_BASE_URL="$PAW_BRAIN_BASE_URL"
export PAW_DRONE_API_KEY="$PAW_BRAIN_API_KEY"
export PAW_DRONE_MODEL=glm-4-flash
```

Run Terminal-Bench 2.0:

```sh
harbor run \
  --dataset terminal-bench@2.0 \
  --agent-import-path paw_harbor:PawAgent \
  --model openai/glm-4.6 \
  --n-concurrent 4
```

Oracle sanity check:

```sh
harbor run \
  --dataset terminal-bench@2.0 \
  --agent oracle
```

`PawAgent` uploads `bin/paw-linux-amd64` to `/usr/local/bin/paw`, writes the task instruction to `/tmp/paw_instruction.txt`, runs in `/workspace`, and keeps scratch output under `/workspace/.paw`.
