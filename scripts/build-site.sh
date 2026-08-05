#!/usr/bin/env bash
# Assemble the GitHub Pages site into dist/site.
# The mark is COPIED from pkg/ui/assets rather than duplicated under site/, so
# the platform assets stay the single source of truth for the logo. The page
# itself is one self-contained HTML file — no build step, no dependencies.
set -euo pipefail
cd "$(dirname "$0")/.."

out=dist/site
rm -rf "$out"
mkdir -p "$out"

cp site/index.html "$out/"
cp pkg/ui/assets/logo.svg pkg/ui/assets/logo-mono.svg "$out/"

# Pages runs Jekyll unless told otherwise; nothing here needs it.
touch "$out/.nojekyll"

echo "ok: site assembled in $out ($(du -sh "$out" | cut -f1))"
