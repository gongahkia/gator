#!/bin/sh
set -eu

days="${NORBOT_VERIFICATION_CACHE_MAX_AGE_DAYS:-30}"
case "$days" in *[!0-9]*|'') echo "NORBOT_VERIFICATION_CACHE_MAX_AGE_DAYS must be a positive integer" >&2; exit 2;; esac
apply=0
[ "${1:-}" = "--apply" ] && apply=1
[ -z "${1:-}" ] || [ "$apply" = 1 ] || { echo "usage: scripts/prune-verification-caches.sh [--apply]" >&2; exit 2; }
now="$(date +%s)"
max_age="$((days * 86400))"

docker volume ls -q | while IFS= read -r volume; do
  case "$volume" in norbot_verify_node_*|norbot_verify_go_*) ;; *) continue;; esac
  case "$volume" in *[!a-zA-Z0-9_.-]*) continue;; esac
  last="$(docker run --rm -v "$volume:/cache:ro" busybox:1.37.0 sh -c 'stat -c %Y /cache/.norbot-last-used 2>/dev/null || echo 0' 2>/dev/null || echo 0)"
  case "$last" in *[!0-9]*|'') last=0;; esac
  age="$((now - last))"
  [ "$last" -gt 0 ] && [ "$age" -gt "$max_age" ] || continue
  if [ "$apply" = 0 ]; then
    echo "would remove $volume (unused for $((age / 86400)) days)"
    continue
  fi
  if docker ps -aq --filter "volume=$volume" | grep -q .; then
    echo "skip in-use volume $volume" >&2
    continue
  fi
  docker volume rm "$volume"
done
