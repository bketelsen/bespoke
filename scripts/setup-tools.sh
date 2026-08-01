#!/usr/bin/env bash
# One-time: install the UI toolchain into ./tools (gitignored).
# Needed only to CHANGE the design system or views, not to build or deploy.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p tools

GOBIN="$PWD/tools" go install github.com/a-h/templ/cmd/templ@latest
GOBIN="$PWD/tools" go install github.com/templui/templui/cmd/templui@latest

arch=$(uname -m); case "$arch" in x86_64) tw=tailwindcss-linux-x64 ;; aarch64) tw=tailwindcss-linux-arm64 ;; *) echo "unsupported arch $arch"; exit 1 ;; esac
curl -sLo tools/tailwindcss "https://github.com/tailwindlabs/tailwindcss/releases/latest/download/$tw"
chmod +x tools/tailwindcss

echo "ok:"; ls tools/
