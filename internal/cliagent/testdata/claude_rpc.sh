#!/bin/sh
# Deterministic Claude streaming protocol fixture, without a model or credential.
set -eu
session=''
for arg in "$@"; do
  case "$arg" in
    --session-id=*) session=${arg#--session-id=}; printf '%s' "$session" > "$HOME/session" ;;
    --resume=*) session=${arg#--resume=}; test "$(cat "$HOME/session")" = "$session" ;;
  esac
done
test -n "$session"
while IFS= read -r request; do
  case "$request" in
    *'"type":"control_request"'*)
      id=$(printf '%s' "$request" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
      printf '{"type":"control_response","response":{"subtype":"success","request_id":"%s","response":{}}}\n' "$id" ;;
    *'"type":"user"'*)
      count=0
      if test -f "$HOME/count"; then count=$(cat "$HOME/count"); fi
      count=$((count + 1))
      printf '%s' "$count" > "$HOME/count"
      printf 'after-%s\n' "$count" > note.txt
      printf '{"type":"result","uuid":"result-%s","session_id":"%s","subtype":"success","is_error":false,"result":"Fixture turn %s"}\n' "$count" "$session" "$count" ;;
  esac
done
