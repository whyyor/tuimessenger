#!/bin/sh
# Installs tuimessenger from its GitHub releases:
#   curl -fsSL https://raw.githubusercontent.com/whyyor/tuimessenger/main/install.sh | sh
# TUIMESSENGER_VERSION=v0.1.0 picks a release (default: the latest); TUIMESSENGER_INSTALL_DIR where the binary goes
# (default: ~/.local/bin). No sudo. Run it again to update; delete the binary to uninstall.
set -eu

repo=whyyor/tuimessenger
dir=${TUIMESSENGER_INSTALL_DIR:-$HOME/.local/bin}

fail() {
	echo "tuimessenger install: $*" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || fail "needs $1"
}
need curl
need tar
need uname

case $(uname -s) in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) fail "no build for $(uname -s); see https://github.com/$repo#building" ;;
esac
case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "no build for $(uname -m) CPUs; see https://github.com/$repo#building" ;;
esac

if [ "$os" = linux ]; then
	# The Linux builds need glibc 2.35 or newer (Ubuntu 22.04, Debian 12, Raspberry Pi OS and later).
	glibc=$(getconf GNU_LIBC_VERSION 2>/dev/null | awk '{print $2}') || true
	if [ -z "$glibc" ]; then
		fail "needs glibc (musl systems such as Alpine aren't supported); see https://github.com/$repo#building"
	fi
	major=${glibc%%.*}
	minor=${glibc#*.}
	minor=${minor%%.*}
	if [ "$major" -lt 2 ] || { [ "$major" -eq 2 ] && [ "$minor" -lt 35 ]; }; then
		fail "needs glibc 2.35 or newer, this system has $glibc"
	fi
fi

version=${TUIMESSENGER_VERSION:-}
if [ -z "$version" ]; then
	# The latest release's page redirects to its tag; no API call, so no rate limit.
	version=$(curl -fsSI "https://github.com/$repo/releases/latest" | tr -d '\r' |
		sed -n 's|^[Ll]ocation: .*/releases/tag/\(.*\)$|\1|p')
	[ -n "$version" ] || fail "couldn't find the latest release"
fi

name="tuimessenger_${version}_${os}_${arch}"
base="https://github.com/$repo/releases/download/$version"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading tuimessenger $version for ${os}/${arch}..."
curl -fsSL -o "$tmp/$name.tar.gz" "$base/$name.tar.gz" || fail "couldn't download $base/$name.tar.gz"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fail "couldn't download the checksums"

want=$(awk -v f="$name.tar.gz" '$2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || fail "no checksum for $name.tar.gz"
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$tmp/$name.tar.gz" | awk '{print $1}')
else
	need shasum
	got=$(shasum -a 256 "$tmp/$name.tar.gz" | awk '{print $1}')
fi
[ "$got" = "$want" ] || fail "checksum mismatch for $name.tar.gz: the download is broken or was tampered with"

tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
mkdir -p "$dir"
# Replace by renaming, so a running tuimessenger keeps its old file until it restarts.
cp "$tmp/$name/tuimessenger" "$dir/.tuimessenger.new"
chmod 755 "$dir/.tuimessenger.new"
mv -f "$dir/.tuimessenger.new" "$dir/tuimessenger"

echo "Installed $("$dir/tuimessenger" version)"
echo "  to $dir/tuimessenger"
case ":$PATH:" in
*":$dir:"*) ;;
*)
	echo
	echo "$dir isn't on your PATH. Add it, e.g. in ~/.bashrc or ~/.zshrc:"
	echo "  export PATH=\"$dir:\$PATH\""
	;;
esac
echo
echo "Start with: tuimessenger tui"
echo "Update: run this again. Uninstall: rm $dir/tuimessenger"
