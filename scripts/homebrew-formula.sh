#!/bin/sh
# Prints the Homebrew formula for a release, e.g. scripts/homebrew-formula.sh v0.1.0 dist/checksums.txt
# The release workflow commits its output to whyyor/homebrew-tap as Formula/tuimessenger.rb.
set -eu

tag=$1
sums=$2
version=${tag#v}
base="https://github.com/whyyor/tuimessenger/releases/download/$tag"

# sha prints the checksum of one release archive, failing if it's missing.
sha() {
	s=$(awk -v f="tuimessenger_${tag}_$1.tar.gz" '$2 == f { print $1 }' "$sums")
	if [ -z "$s" ]; then
		echo "no checksum for tuimessenger_${tag}_$1.tar.gz in $sums" >&2
		exit 1
	fi
	echo "$s"
}

darwin_arm64=$(sha darwin_arm64)
darwin_amd64=$(sha darwin_amd64)
linux_arm64=$(sha linux_arm64)
linux_amd64=$(sha linux_amd64)

cat <<EOF
# Written by scripts/homebrew-formula.sh in whyyor/tuimessenger for every release; changes made here are overwritten.
class Tuimessenger < Formula
  desc "WhatsApp, Telegram, Slack and Instagram in one terminal app"
  homepage "https://github.com/whyyor/tuimessenger"
  version "$version"
  license "AGPL-3.0-only"

  on_macos do
    on_arm do
      url "$base/tuimessenger_${tag}_darwin_arm64.tar.gz"
      sha256 "$darwin_arm64"
    end
    on_intel do
      url "$base/tuimessenger_${tag}_darwin_amd64.tar.gz"
      sha256 "$darwin_amd64"
    end
  end

  on_linux do
    on_arm do
      url "$base/tuimessenger_${tag}_linux_arm64.tar.gz"
      sha256 "$linux_arm64"
    end
    on_intel do
      url "$base/tuimessenger_${tag}_linux_amd64.tar.gz"
      sha256 "$linux_amd64"
    end
  end

  def install
    bin.install "tuimessenger"
  end

  def caveats
    <<~EOS
      Start with:
        tuimessenger tui

      If you run the backend as a service, restart it after upgrading:
        tuimessenger service install
    EOS
  end

  test do
    assert_match "tuimessenger v#{version} ", shell_output("#{bin}/tuimessenger version")
  end
end
EOF
