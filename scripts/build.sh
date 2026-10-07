#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p dist
if [ "$(uname -s)" = Darwin ]; then
  xcrun swiftc -O -target arm64-apple-macos13 native/macos/MultiCodexHelper.swift -o dist/helper-arm64
  xcrun swiftc -O -target x86_64-apple-macos13 native/macos/MultiCodexHelper.swift -o dist/helper-amd64
  lipo -create dist/helper-arm64 dist/helper-amd64 -output cmd/multi-codex-app/assets/macos/MultiCodexHelper
fi
go build -trimpath -ldflags "-s -w -X main.version=${MULTI_CODEX_BUILD_VERSION:-dev}" -o dist/multi-codex-app ./cmd/multi-codex-app
