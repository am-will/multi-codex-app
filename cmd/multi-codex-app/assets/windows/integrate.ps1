param([Parameter(Mandatory=$true)][string]$ConfigPath, [switch]$Uninstall)
$ErrorActionPreference = 'Stop'
$config = Get-Content -LiteralPath $ConfigPath -Raw | ConvertFrom-Json
$root = Split-Path -Parent $ConfigPath
$backup = Join-Path $root 'codex-handler-backup.reg'
$key = 'HKCU:\Software\Classes\codex'
$menu = Join-Path ([Environment]::GetFolderPath('Programs')) 'Multi Codex'
if ($Uninstall) {
    if ((Test-Path $key) -and ((Get-ItemProperty "$key\shell\open\command").'(default)' -like '*multi-codex-app.exe*')) {
        Remove-Item -LiteralPath $key -Recurse
        if (Test-Path $backup) { & reg.exe import $backup | Out-Null; if ($LASTEXITCODE -ne 0) { throw 'Registry restore failed' } }
    }
    if (Test-Path $menu) { Remove-Item -LiteralPath $menu -Recurse }
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
    $shortcut = $shell.CreateShortcut((Join-Path $menu ('Codex Profile ' + $profile.id + '.lnk')))
    $shortcut.TargetPath = $config.cliPath
    $shortcut.Arguments = '--root "' + $root + '" launch ' + $profile.id
    $shortcut.Description = $profile.name
    $shortcut.WorkingDirectory = $root
    $shortcut.Save()
}
$bin = Split-Path -Parent $config.cliPath
$userPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
if (($userPath -split ';') -notcontains $bin) { [Environment]::SetEnvironmentVariable('PATH', "$userPath;$bin", 'User') }
Write-Output 'Start-menu shortcuts and per-user codex:// registration installed. If Windows retains another default, select Multi Codex in Default Apps. Open a new terminal to use the CLI.'
