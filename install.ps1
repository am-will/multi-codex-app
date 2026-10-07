param([int]$Count = 0, [string]$App = '')
$ErrorActionPreference = 'Stop'
$arch = if ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }
$asset = "multi-codex-app_windows_$arch.zip"
$base = if ($env:MULTI_CODEX_VERSION) { "https://github.com/am-will/multi-codex-app/releases/download/$env:MULTI_CODEX_VERSION" } else { 'https://github.com/am-will/multi-codex-app/releases/latest/download' }
$temp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $temp | Out-Null
try {
    Invoke-WebRequest "$base/$asset" -OutFile (Join-Path $temp $asset) -UseBasicParsing
    $sum = (Invoke-WebRequest "$base/$asset.sha256" -UseBasicParsing).Content.Trim().Split(' ')[0]
    if ((Get-FileHash (Join-Path $temp $asset) -Algorithm SHA256).Hash -ne $sum) { throw 'Release checksum mismatch' }
    Expand-Archive -LiteralPath (Join-Path $temp $asset) -DestinationPath $temp
    $options = @('setup')
    if ($Count -gt 0) { $options += @('--count', "$Count") }
    if ($App) { $options += @('--app', $App) }
    & (Join-Path $temp 'multi-codex-app.exe') @options
    if ($LASTEXITCODE -ne 0) { throw 'Multi Codex setup failed' }
} finally { Remove-Item -LiteralPath $temp -Recurse -Force }
