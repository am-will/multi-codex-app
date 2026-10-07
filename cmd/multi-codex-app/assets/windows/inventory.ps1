# Read existing shortcuts and registration. No Save(), registry writes, or app launch.
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
$menu = Join-Path ([Environment]::GetFolderPath('Programs')) 'Multi Codex'
$items = @()
if (Test-Path -LiteralPath $menu) {
    $shell = New-Object -ComObject WScript.Shell
    Get-ChildItem -LiteralPath $menu -Filter '*.lnk' | ForEach-Object {
        $shortcut = $shell.CreateShortcut($_.FullName)
        $items += [PSCustomObject]@{
            Name = $_.BaseName
            Path = $_.FullName
            Target = $shortcut.TargetPath
            Arguments = $shortcut.Arguments
            Icon = $shortcut.IconLocation
        }
    }
}
$key = 'HKCU:\Software\Classes\codex\shell\open\command'
$helper = $false
if (Test-Path -LiteralPath $key) {
    $command = (Get-ItemProperty -LiteralPath $key).'(default)'
    $helper = $command -like '*multi-codex-app.exe*callback*'
}
[PSCustomObject]@{ helper = $helper; shortcuts = @($items) } | ConvertTo-Json -Depth 4 -Compress
