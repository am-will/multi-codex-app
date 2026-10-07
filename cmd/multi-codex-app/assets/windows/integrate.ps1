param([Parameter(Mandatory=$true)][string]$ConfigPath, [switch]$Uninstall, [string]$RemoveProfileID = '')
$ErrorActionPreference = 'Stop'
$config = Get-Content -LiteralPath $ConfigPath -Raw | ConvertFrom-Json
$root = Split-Path -Parent $ConfigPath
$backup = Join-Path $root 'codex-handler-backup.reg'
$key = 'HKCU:\Software\Classes\codex'
$menu = Join-Path ([Environment]::GetFolderPath('Programs')) 'Multi Codex'
if ($RemoveProfileID) {
    $shell = New-Object -ComObject WScript.Shell
    $arguments = '--root "' + $root + '" launch ' + $RemoveProfileID
    if (Test-Path -LiteralPath $menu) {
        Get-ChildItem -LiteralPath $menu -Filter '*.lnk' | ForEach-Object {
            $shortcut = $shell.CreateShortcut($_.FullName)
            if ($shortcut.TargetPath -eq $config.cliPath -and $shortcut.Arguments -eq $arguments) {
                $retired = Join-Path $root 'retired-launchers'
                New-Item -ItemType Directory -Path $retired -Force | Out-Null
                Move-Item -LiteralPath $_.FullName -Destination (Join-Path $retired (([Guid]::NewGuid().ToString()) + '.lnk'))
            }
        }
    }
    return
}
if ($Uninstall) {
    $ownedCommand = '"' + $config.cliPath + '" --root "' + $root + '" callback "%1"'
    if ((Test-Path "$key\shell\open\command") -and ((Get-ItemProperty "$key\shell\open\command").'(default)' -eq $ownedCommand)) {
        Remove-Item -LiteralPath $key -Recurse
        if (Test-Path $backup) { & reg.exe import $backup | Out-Null; if ($LASTEXITCODE -ne 0) { throw 'Registry restore failed' } }
    }
    if (Test-Path -LiteralPath $menu) {
        $shell = New-Object -ComObject WScript.Shell
        $prefix = '--root "' + $root + '" launch '
        Get-ChildItem -LiteralPath $menu -Filter '*.lnk' | ForEach-Object {
            $shortcut = $shell.CreateShortcut($_.FullName)
            if ($shortcut.TargetPath -eq $config.cliPath -and $shortcut.Arguments.StartsWith($prefix)) { Remove-Item -LiteralPath $_.FullName }
        }
        if (-not (Get-ChildItem -LiteralPath $menu)) { Remove-Item -LiteralPath $menu }
    }
    return
}
if (-not (Test-Path (Join-Path $root 'registry-backup-made'))) {
    if (Test-Path $key) { & reg.exe export 'HKCU\Software\Classes\codex' $backup /y | Out-Null; if ($LASTEXITCODE -ne 0) { throw 'Registry backup failed' } }
    New-Item -ItemType File -Path (Join-Path $root 'registry-backup-made') | Out-Null
}
New-Item -Path "$key\shell\open\command" -Force | Out-Null
Set-Item -Path $key -Value 'URL:Multi Codex'
New-ItemProperty -Path $key -Name 'URL Protocol' -Value '' -Force | Out-Null
$command = '"' + $config.cliPath + '" --root "' + $root + '" callback "%1"'
Set-Item -Path "$key\shell\open\command" -Value $command
New-Item -ItemType Directory -Path $menu -Force | Out-Null
$shell = New-Object -ComObject WScript.Shell
foreach ($profile in $config.profiles) {
    $displayName = $profile.name.Trim()
    if ($displayName -notmatch '^Codex[ (]') { $displayName = 'Codex ' + $displayName }
    $destination = Join-Path $menu ($displayName + '.lnk')
    $arguments = '--root "' + $root + '" launch ' + $profile.id
    if (Test-Path -LiteralPath $destination) {
        $existing = $shell.CreateShortcut($destination)
        if ($existing.TargetPath -ne $config.cliPath -or $existing.Arguments -ne $arguments) { throw "Another shortcut already uses $displayName" }
    }
    # Retire earlier shortcuts for this same CLI/profile, including old numbered names.
    Get-ChildItem -LiteralPath $menu -Filter '*.lnk' | ForEach-Object {
        $existing = $shell.CreateShortcut($_.FullName)
        if ($_.FullName -ne $destination -and $existing.TargetPath -eq $config.cliPath -and $existing.Arguments -eq $arguments) {
            $retired = Join-Path $root 'retired-launchers'
            New-Item -ItemType Directory -Path $retired -Force | Out-Null
            Move-Item -LiteralPath $_.FullName -Destination (Join-Path $retired (([Guid]::NewGuid().ToString()) + '.lnk'))
        }
    }
    $shortcut = $shell.CreateShortcut($destination)
    $shortcut.TargetPath = $config.cliPath
    $shortcut.Arguments = $arguments
    $shortcut.Description = $profile.name
    $shortcut.IconLocation = (Join-Path $root ('icons\' + $profile.iconColor + '.ico')) + ',0'
    $shortcut.WorkingDirectory = $root
    $shortcut.Save()
}
$bin = Split-Path -Parent $config.cliPath
$userPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
if (($userPath -split ';') -notcontains $bin) { [Environment]::SetEnvironmentVariable('PATH', "$userPath;$bin", 'User') }
Write-Output 'Start-menu shortcuts and per-user codex:// registration installed. If Windows retains another default, select Multi Codex in Default Apps. Open a new terminal to use the CLI.'
