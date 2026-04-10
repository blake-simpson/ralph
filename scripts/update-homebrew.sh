#!/bin/bash
set -e

# Update the Homebrew tap formula with new version and checksums.
#
# Usage:
#   ./scripts/update-homebrew.sh <version> <binaries-dir>
#
# Arguments:
#   version       Release version (e.g. 0.8.7)
#   binaries-dir  Directory containing the release binaries
#
# Environment:
#   HOMEBREW_TAP_TOKEN  GitHub token with push access to the tap repo
#   HOMEBREW_TAP_REPO   Tap repo (default: blake-simpson/homebrew-belmont)
#
# Called from CI after binaries are uploaded to the GitHub Release.

if [ -z "$1" ] || [ -z "$2" ]; then
    echo "Usage: ./scripts/update-homebrew.sh <version> <binaries-dir>"
    exit 1
fi

VERSION="$1"
BIN_DIR="$2"
TAP_REPO="${HOMEBREW_TAP_REPO:-blake-simpson/homebrew-belmont}"

if [ -z "$HOMEBREW_TAP_TOKEN" ]; then
    echo "Error: HOMEBREW_TAP_TOKEN environment variable is required"
    exit 1
fi

# Compute SHA-256 checksums for each platform binary
sha_darwin_arm64=$(sha256sum "$BIN_DIR/belmont-darwin-arm64" | awk '{print $1}')
sha_darwin_amd64=$(sha256sum "$BIN_DIR/belmont-darwin-amd64" | awk '{print $1}')
sha_linux_arm64=$(sha256sum "$BIN_DIR/belmont-linux-arm64" | awk '{print $1}')
sha_linux_amd64=$(sha256sum "$BIN_DIR/belmont-linux-amd64" | awk '{print $1}')

echo "Checksums:"
echo "  darwin-arm64: $sha_darwin_arm64"
echo "  darwin-amd64: $sha_darwin_amd64"
echo "  linux-arm64:  $sha_linux_arm64"
echo "  linux-amd64:  $sha_linux_amd64"

# Generate the formula
FORMULA=$(cat <<RUBY
class Belmont < Formula
  desc "Structured AI coding sessions with PRD-driven planning and verification"
  homepage "https://github.com/blake-simpson/belmont"
  version "$VERSION"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/blake-simpson/belmont/releases/download/v#{version}/belmont-darwin-arm64"
      sha256 "$sha_darwin_arm64"
    else
      url "https://github.com/blake-simpson/belmont/releases/download/v#{version}/belmont-darwin-amd64"
      sha256 "$sha_darwin_amd64"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/blake-simpson/belmont/releases/download/v#{version}/belmont-linux-arm64"
      sha256 "$sha_linux_arm64"
    else
      url "https://github.com/blake-simpson/belmont/releases/download/v#{version}/belmont-linux-amd64"
      sha256 "$sha_linux_amd64"
    end
  end

  def install
    binary = Dir["belmont-*"].first
    bin.install binary => "belmont"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/belmont version")
  end
end
RUBY
)

# Clone the tap repo, update the formula, and push
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

echo ""
echo "Cloning tap repo..."
git clone "https://x-access-token:${HOMEBREW_TAP_TOKEN}@github.com/${TAP_REPO}.git" "$WORK_DIR/tap"

cd "$WORK_DIR/tap"
mkdir -p Formula
echo "$FORMULA" > Formula/belmont.rb

git add Formula/belmont.rb
if git diff --cached --quiet; then
    echo "Formula is already up to date."
    exit 0
fi

git config user.name "github-actions[bot]"
git config user.email "github-actions[bot]@users.noreply.github.com"
git commit -m "Update belmont to v${VERSION}"
git push origin main

echo ""
echo "Homebrew tap updated to v${VERSION}"
