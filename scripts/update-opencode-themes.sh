#!/bin/sh
# Downloads opencode's built-in TUI themes (MIT) into internal/tui/themes/opencode at a pinned commit.
# Bump SHA to update them, rerun, and review the diff.
set -eu

SHA=0bd8cee7c2c77d04cad50dace81520c34c805c3d
BASE="https://raw.githubusercontent.com/anomalyco/opencode/$SHA"
THEMES="aura ayu carbonfox catppuccin catppuccin-frappe catppuccin-macchiato cobalt2 cursor dracula everforest
flexoki github gruvbox kanagawa lucent-orng material matrix mercury monokai nightowl nord one-dark opencode orng
osaka-jade palenight rosepine solarized synthwave84 tokyonight vercel vesper zenburn"

dir="$(cd "$(dirname "$0")/.." && pwd)/internal/tui/themes/opencode"
rm -rf "$dir"
mkdir -p "$dir"
curl -fsSL "$BASE/LICENSE" -o "$dir/LICENSE"
for name in $THEMES; do
	curl -fsSL "$BASE/packages/tui/src/theme/assets/$name.json" -o "$dir/$name.json"
done
echo "$SHA" > "$dir/COMMIT"
echo "downloaded $(ls "$dir"/*.json | wc -l | tr -d ' ') themes from anomalyco/opencode@$SHA"
