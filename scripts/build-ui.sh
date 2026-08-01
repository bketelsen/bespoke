#!/usr/bin/env bash
# Regenerate templ views and recompile the design system CSS.
# Run after changing any .templ file or design/input.css. Outputs are
# COMMITTED (generated *_templ.go and pkg/ui/assets/styles.css), so deploys
# and fresh clones need no UI toolchain — only edits here do.
# Tools come from scripts/setup-tools.sh (./tools, gitignored).
set -euo pipefail
cd "$(dirname "$0")/.."

./tools/templ generate
./tools/tailwindcss -i design/input.css -o pkg/ui/assets/styles.css --minify
echo "ok: templ generated, pkg/ui/assets/styles.css $(wc -c < pkg/ui/assets/styles.css) bytes"
