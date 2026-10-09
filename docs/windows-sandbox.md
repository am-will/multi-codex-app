# Windows sandbox setup in a secondary profile

A secondary profile has its own `CODEX_HOME`. If its desktop app shows **"Windows setup didn't finish"** while the original app works, the two profiles may be using different Windows sandbox preferences. A classic sandbox setup can also fail when another Codex instance has a runtime file open. This is a Codex sandbox setup error, not a sign-in or profile-isolation failure in Multi Codex App.

On a compatible device, Codex can prefer Microsoft Execution Containers (MXC) without administrator setup. Codex still enforces the active permission profile and managed requirements, and falls back to the configured classic implementation when MXC is unavailable or disallowed. Test compatibility before changing a profile's saved preference; do not disable the sandbox to dismiss the banner. See the [official Windows sandbox guide](https://learn.chatgpt.com/docs/windows/windows-sandbox) for compatibility limits and organizational policy.

1. Quit only the affected desktop profile. Use `multi-codex-app list` to find its number. In PowerShell, select that profile's Codex home, replacing `2` with its number:

   ```powershell
   $root = Join-Path $env:LOCALAPPDATA 'MultiCodex'
   $config = Get-Content -LiteralPath (Join-Path $root 'config.json') -Raw | ConvertFrom-Json
   $profile = $config.profiles | Where-Object { $_.id -eq '2' } | Select-Object -First 1
   if (-not $profile) { throw 'Profile not found' }
   ```

2. With the official Codex CLI installed, run the [one-command MXC compatibility check](https://learn.chatgpt.com/docs/windows/windows-sandbox#mxc-compatibility) for that profile. This command does not change its saved sandbox selection:

   ```powershell
   $previousCodexHome = $env:CODEX_HOME
   try {
       $env:CODEX_HOME = $profile.codexHome
       codex -c windows.sandbox=mxc sandbox --include-managed-config --permission-profile :workspace -- cmd.exe /d /c echo MXC_OK
       $probeExitCode = $LASTEXITCODE
   } finally {
       $env:CODEX_HOME = $previousCodexHome
   }
   $probeExitCode
   ```

   Continue only if the command prints `MXC_OK` and the exit code is `0`. Also validate the file and network access your work needs, as described in the official guide. If the check fails or your organization disallows MXC, use the supported classic setup or ask your administrator.

3. Back up and open only this profile's `config.toml`:

   ```powershell
   $configPath = Join-Path $profile.codexHome 'config.toml'
   if (Test-Path -LiteralPath $configPath) {
       $backupPath = $configPath + '.' + [guid]::NewGuid().ToString('N') + '.bak'
       Copy-Item -LiteralPath $configPath -Destination $backupPath
       Write-Output "Backup: $backupPath"
   }
   notepad.exe $configPath
   ```

   Add `prefer_mxc = true` under an existing `[features]` table, or add this table if none exists:

   ```toml
   [features]
   prefer_mxc = true
   ```

   Keep any existing `windows.sandbox` fallback and other settings. Save the file and reopen only the affected desktop profile. This changes only that profile's Codex configuration; the original profile and other accounts retain their own settings.
