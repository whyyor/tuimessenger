#!/bin/sh
# Recomputes vendorHash in flake.nix after go.mod/go.sum change (e.g. a Dependabot bump); the flake fails to build
# with a stale hash. Needs nix with flakes.
set -eu
cd "$(dirname "$0")/.."
old=$(sed -n 's/.*vendorHash = "\(sha256-[^"]*\)".*/\1/p' flake.nix)
fake=sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
sed -i.bak "s|$old|$fake|" flake.nix && rm -f flake.nix.bak
new=$(nix build .#tuimessenger --no-link 2>&1 | sed -n 's/.*got: *\(sha256-[^ ]*\).*/\1/p' | head -1)
if [ -z "$new" ]; then
	sed -i.bak "s|$fake|$old|" flake.nix && rm -f flake.nix.bak
	echo "vendorHash unchanged ($old)"
	exit 0
fi
sed -i.bak "s|$fake|$new|" flake.nix && rm -f flake.nix.bak
echo "vendorHash: $old -> $new"
