#!/usr/bin/env bash
set -euo pipefail

PAW_BIN="${1:-paw}"
if [[ "$PAW_BIN" != /* ]]; then
  PAW_BIN="$(pwd)/$PAW_BIN"
fi
ROOT="$(mktemp -d)"
SERVER_PID=""

cleanup() {
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$ROOT"
}
trap cleanup EXIT

cd "$ROOT"

cat > fake_openai.py <<'PY'
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

port_file = sys.argv[1]

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("content-length", "0"))
        request = json.loads(self.rfile.read(length))
        prompt = "\n".join(m.get("content", "") for m in request.get("messages", []))
        if "Target path:" in prompt:
            content = "\n".join([
                "--- a/calc.go",
                "+++ b/calc.go",
                "@@ -1,5 +1,5 @@",
                " package demo",
                " ",
                " func Add(a int, b int) int {",
                "-\treturn a - b",
                "+\treturn a + b",
                " }",
                "",
            ])
        else:
            content = json.dumps({
                "done": False,
                "reasoning": "The failing test expects Add to add, not subtract.",
                "next_action": {
                    "kind": "edit_file",
                    "description": "Change Add to return a + b.",
                    "target_path": "calc.go",
                },
            })
        body = json.dumps({
            "choices": [{"message": {"content": content}}],
            "usage": {"prompt_tokens": 80, "completion_tokens": 24},
        }).encode()
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        return

server = HTTPServer(("127.0.0.1", 0), Handler)
with open(port_file, "w", encoding="utf-8") as f:
    f.write(str(server.server_port))
server.serve_forever()
PY

python3 fake_openai.py "$ROOT/port" >/dev/null 2>&1 &
SERVER_PID="$!"
for _ in {1..50}; do
  [[ -s "$ROOT/port" ]] && break
  sleep 0.1
done
PORT="$(cat "$ROOT/port")"

cat > go.mod <<'EOF'
module demo

go 1.23
EOF

cat > calc.go <<'EOF'
package demo

func Add(a int, b int) int {
	return a - b
}
EOF

cat > calc_test.go <<'EOF'
package demo

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(2, 3); got != 5 {
		t.Fatalf("Add(2, 3) = %d", got)
	}
}
EOF

cat > config.toml <<EOF
max_turns = 2

[brain]
transport = "openai"
base_url = "http://127.0.0.1:${PORT}/v1"
model = "demo-brain"

[policy.provider]
allowed_transports = ["openai"]
allow_loopback = true
EOF

echo '$ go test ./...'
go test ./... || true
echo
echo '$ paw run --raw-context --instruction "fix the failing test"'
PAW_VERIFY_CMD="go test ./..." "$PAW_BIN" --config config.toml run --raw-context --quiet --instruction "fix the failing test" > result.json
echo "review session:"
"$PAW_BIN" session list
echo
echo '$ sed -n "1,6p" calc.go'
sed -n "1,6p" calc.go
