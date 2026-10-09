param([Parameter(Mandatory=$true)][string]$ConfigPath, [Parameter(Mandatory=$true)][string]$ProfileID)
$ErrorActionPreference = 'Stop'

# Windows uses the running window's AppUserModel properties for its taskbar
# icon. The Start-menu shortcut's IconLocation alone does not affect it.
Add-Type @'
using System;
using System.Runtime.InteropServices;

[StructLayout(LayoutKind.Sequential)]
public struct PropertyKey {
    public Guid FormatId;
    public uint PropertyId;
    public PropertyKey(Guid formatId, uint propertyId) { FormatId = formatId; PropertyId = propertyId; }
}

[StructLayout(LayoutKind.Explicit, Size = 24)]
public struct PropertyValue {
    [FieldOffset(0)] public ushort Type;
    [FieldOffset(8)] public IntPtr Text;
}

[ComImport, Guid("886D8EEB-8CF2-4446-8D02-CDBA1DBDCF99"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
public interface IWindowPropertyStore {
    [PreserveSig] int GetCount(out uint count);
    [PreserveSig] int GetAt(uint index, out PropertyKey key);
    [PreserveSig] int GetValue(ref PropertyKey key, out PropertyValue value);
    [PreserveSig] int SetValue(ref PropertyKey key, ref PropertyValue value);
    [PreserveSig] int Commit();
}

public static class ProfileWindowIdentity {
    [DllImport("shell32.dll")]
    private static extern int SHGetPropertyStoreForWindow(IntPtr window, ref Guid interfaceId, out IntPtr store);

    [DllImport("ole32.dll")]
    private static extern int PropVariantClear(ref PropertyValue value);

    private static readonly Guid InterfaceId = new Guid("886D8EEB-8CF2-4446-8D02-CDBA1DBDCF99");
    private static readonly Guid FormatId = new Guid("9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3");

    private static void SetString(IWindowPropertyStore store, uint propertyId, string text) {
        PropertyKey key = new PropertyKey(FormatId, propertyId);
        PropertyValue value = new PropertyValue();
        value.Type = 31; // VT_LPWSTR
        value.Text = Marshal.StringToCoTaskMemUni(text);
        try {
            int result = store.SetValue(ref key, ref value);
            if (result != 0) Marshal.ThrowExceptionForHR(result);
        } finally {
            PropVariantClear(ref value);
        }
    }

    public static void Set(IntPtr window, string appId, string iconResource, string relaunchCommand) {
        Guid interfaceId = InterfaceId;
        IntPtr rawStore;
        int result = SHGetPropertyStoreForWindow(window, ref interfaceId, out rawStore);
        if (result != 0) Marshal.ThrowExceptionForHR(result);
        try {
            IWindowPropertyStore store = (IWindowPropertyStore)Marshal.GetTypedObjectForIUnknown(rawStore, typeof(IWindowPropertyStore));
            SetString(store, 3, iconResource); // RelaunchIconResource
            SetString(store, 2, relaunchCommand); // RelaunchCommand
            SetString(store, 5, appId); // AppUserModelID, set last to refresh the taskbar
            result = store.Commit();
            if (result != 0) Marshal.ThrowExceptionForHR(result);
        } finally {
            Marshal.Release(rawStore);
        }
    }
}
'@

$config = Get-Content -LiteralPath $ConfigPath -Raw | ConvertFrom-Json
$profile = $config.profiles | Where-Object { $_.id -eq $ProfileID } | Select-Object -First 1
if (-not $profile -or -not $profile.userDataDir) { return }

$root = Split-Path -Parent $ConfigPath
$icon = Join-Path $root ('icons\' + $profile.iconColor + '.ico')
if (-not (Test-Path -LiteralPath $icon -PathType Leaf)) { return }

$argument = '--user-data-dir=' + $profile.userDataDir
$exeName = [IO.Path]::GetFileName($config.appPath).Replace("'", "''")
$deadline = (Get-Date).AddSeconds(30)
$window = [IntPtr]::Zero
do {
    $process = Get-CimInstance Win32_Process -Filter "Name='$exeName'" |
        Where-Object {
            $_.CommandLine -and $_.CommandLine.Contains($argument) -and
            $_.CommandLine -notmatch '--type='
        } | Select-Object -First 1
    if ($process) {
        $running = Get-Process -Id $process.ProcessId -ErrorAction SilentlyContinue
        if ($running) {
            $window = $running.MainWindowHandle
            if ($window -ne [IntPtr]::Zero) { break }
        }
    }
    Start-Sleep -Milliseconds 500
} while ((Get-Date) -lt $deadline)
if ($window -eq [IntPtr]::Zero) { return }

$relaunch = '"' + $config.cliPath + '" --root "' + $root + '" launch ' + $ProfileID
[ProfileWindowIdentity]::Set($window, ('MultiCodex.Profile.' + $ProfileID), ($icon + ',0'), $relaunch)
