#!/usr/bin/env bash
# Renders docs/demo/demo.gif from docs/demo/demo.tape with VHS, in Docker, so
# nothing but Docker and Go needs installing. It builds claudes and the fake
# claude from test/e2e for Linux, and the demo runs against made-up sessions.
#
# Usage: hack/demo.sh
set -euo pipefail

vhs=ghcr.io/charmbracelet/vhs:v0.12.1@sha256:ea49a6a1c529be83153e88321892b5585964418f0b9055e8c1e0d732194234a3
root=$(cd "$(dirname "$0")/.." && pwd)
bin=$root/docs/demo/bin

case "$(docker info --format '{{.Architecture}}')" in
aarch64 | arm64) arch=arm64 ;;
*) arch=amd64 ;;
esac

mkdir -p "$bin"
trap 'rm -rf "$bin"' EXIT
(
  cd "$root"
  GOOS=linux GOARCH=$arch CGO_ENABLED=0 go build -o "$bin/claudes" ./cmd/claudes
  GOOS=linux GOARCH=$arch CGO_ENABLED=0 go build -o "$bin/claude" ./test/e2e/fakeclaude
)
docker run --rm -v "$root:/vhs" -w /vhs "$vhs" docs/demo/demo.tape
echo "Wrote docs/demo/demo.gif"
