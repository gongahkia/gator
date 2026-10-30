#!/bin/sh
# Deterministic protocol fixture; never a substitute for live model evidence.
set -eu
while IFS= read -r request; do
  id=$(printf '%s' "$request" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
  case "$request" in
    *'"method":"initialize"'*)
      printf '{"id":"%s","result":{}}\n' "$id" ;;
    *'"method":"thread/start"'*|*'"method":"thread/resume"'*)
      printf '{"id":"%s","result":{"thread":{"id":"fixture-thread"}}}\n' "$id" ;;
    *'"method":"turn/start"'*)
      count=0
      if test -f "$HOME/count"; then count=$(cat "$HOME/count"); fi
      count=$((count + 1))
      printf '%s' "$count" > "$HOME/count"
      printf 'after-%s\n' "$count" > note.txt
      # A child-created Git directory must never become trusted host metadata.
      mkdir -p .git
      printf 'invalid child metadata\n' > .git/config
      printf '{"id":"%s","result":{"turn":{"id":"turn-%s"}}}\n' "$id" "$count"
      printf '{"method":"item/completed","params":{"threadId":"fixture-thread","turnId":"turn-%s","item":{"type":"agentMessage","text":"Updated note.txt on turn %s"}}}\n' "$count" "$count"
      printf '{"method":"turn/completed","params":{"threadId":"fixture-thread","turn":{"id":"turn-%s","status":"completed"}}}\n' "$count" ;;
  esac
done
