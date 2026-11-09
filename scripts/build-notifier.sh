#!/bin/sh
# Builds assets/terminal-notifier.zip, the notification app bundled into tuimessenger for macOS, from a pinned
# terminal-notifier source release (MIT, github.com/julienXX/terminal-notifier). Needs the Xcode command line tools.
# Rerun it to check the committed zip: the app inside should match apart from its signature.
set -eu

VERSION=3.1.0
SHA256=7dac44a563f00c10d49aa2da4cde9d1fecdb12b36ed57fe7fdff789c3578421e
URL="https://github.com/julienXX/terminal-notifier/archive/refs/tags/$VERSION.tar.gz"

root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

curl -fsSL "$URL" -o "$work/src.tgz"
echo "$SHA256  $work/src.tgz" | shasum -a 256 -c -
tar -xzf "$work/src.tgz" -C "$work"
cd "$work/terminal-notifier-$VERSION"
make app >/dev/null # universal (arm64 + x86_64), macOS 10.14+, ad-hoc signed

app=build/terminal-notifier.app
# Apple's Terminal icon isn't covered by the MIT license; tuimessenger installs its own icon.
rm "$app/Contents/Resources/Terminal.icns"
rm -f "$root/assets/terminal-notifier.zip"
(cd build && zip -X -r -q "$root/assets/terminal-notifier.zip" terminal-notifier.app)
echo "built assets/terminal-notifier.zip from terminal-notifier $VERSION:"
lipo -info "$app/Contents/MacOS/terminal-notifier"
