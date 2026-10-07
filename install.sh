#!/bin/sh
# Install a released binary. No Node, Go compiler, or Swift compiler is required.
set -eu
repo='am-will/multi-codex-app'
case "$(uname -s)" in Darwin) platform=darwin ;; Linux) platform=linux ;; *) echo 'Use install.ps1 from PowerShell on Windows.' >&2; exit 1 ;; esac
case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) echo 'Supported architectures: arm64 and amd64.' >&2; exit 1 ;; esac
asset="multi-codex-app_${platform}_${arch}.tar.gz"
if [ -n "${MULTI_CODEX_VERSION:-}" ]; then base="https://github.com/$repo/releases/download/$MULTI_CODEX_VERSION"; else base="https://github.com/$repo/releases/latest/download"; fi
task_temp=$(mktemp -d)
trap 'rm -rf "$task_temp"' EXIT HUP INT TERM
curl --proto '=https' --tlsv1.2 -fsSL "$base/$asset" -o "$task_temp/$asset"
curl --proto '=https' --tlsv1.2 -fsSL "$base/$asset.sha256" -o "$task_temp/$asset.sha256"
(cd "$task_temp"; if command -v sha256sum >/dev/null 2>&1; then sha256sum -c "$asset.sha256"; else shasum -a 256 -c "$asset.sha256"; fi)
# The publisher's release tarball contains only this executable.
tar -xzf "$task_temp/$asset" -C "$task_temp" multi-codex-app
"$task_temp/multi-codex-app" setup "$@"
printf '\nIf the CLI is not on your PATH, add ~/.local/bin to PATH:\n  export PATH="$HOME/.local/bin:$PATH"\n'
