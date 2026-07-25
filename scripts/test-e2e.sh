#!/usr/bin/env bash
set -euo pipefail

cleanup() { docker compose -f docker-compose.e2e.yml down --volumes --remove-orphans; }
trap cleanup EXIT
docker compose -f docker-compose.e2e.yml up -d --wait
NORBOT_TEST_DATABASE_URL='postgres://norbot:norbot@127.0.0.1:55432/norbot?sslmode=disable' go test -race ./internal/store
NORBOT_E2E_DOCKER=1 go test -race -run TestGeneratedProfileDockerE2E ./internal/engine
