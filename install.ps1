# jbn-player installer. Usage (PowerShell):
#   irm https://raw.githubusercontent.com/MrT-coder/jbn-player/main/install.ps1 | iex
# Installs to %LOCALAPPDATA%\Programs\jbn-player, no admin needed. Re-run to update.
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$repo    = "MrT-coder/jbn-player"
$dest    = Join-Path $env:LOCALAPPDATA "Programs\jbn-player"
$tmp     = Join-Path $env:TEMP ("jbn-player-" + [guid]::NewGuid().ToString("N"))
$headers = @{ "User-Agent" = "jbn-player-installer" }

function Get-Asset($ownerRepo, $pattern) {
    $rel = Invoke-RestMethod "https://api.github.com/repos/$ownerRepo/releases/latest" -Headers $headers
    $a = $rel.assets | Where-Object { $_.name -like $pattern } | Select-Object -First 1
    if (-not $a) { throw "No asset matching '$pattern' in $ownerRepo latest release." }
    $a
}

New-Item -ItemType Directory -Force $tmp, $dest | Out-Null
try {
    Get-Process jbn-player -ErrorAction SilentlyContinue | Stop-Process -Force
    Start-Sleep -Milliseconds 500

    Write-Host "Downloading jbn-player..."
    $app = Get-Asset $repo "jbn-player-windows-x64*.zip"
    $zip = Join-Path $tmp $app.name
    Invoke-WebRequest $app.browser_download_url -OutFile $zip -Headers $headers
    Expand-Archive $zip -DestinationPath $dest -Force

    # libmpv is not redistributed here: fetch it from its own build project.
    $dll = Join-Path $dest "libmpv-2.dll"
    if (-not (Test-Path $dll)) {
        Write-Host "Downloading libmpv (audio engine, ~30 MB)..."
        $mpv = Get-Asset "shinchiro/mpv-winbuild-cmake" "mpv-dev-x86_64-2*.7z"
        $z7  = Join-Path $tmp $mpv.name
        Invoke-WebRequest $mpv.browser_download_url -OutFile $z7 -Headers $headers
        # Windows 11 tar (libarchive) reads .7z natively.
        & "$env:SystemRoot\System32\tar.exe" -xf $z7 -C $dest "libmpv-2.dll"
        if ($LASTEXITCODE -ne 0 -or -not (Test-Path $dll)) {
            throw "Could not extract libmpv-2.dll. Put it next to jbn-player.exe in $dest by hand."
        }
    }

    $lnk = Join-Path ([Environment]::GetFolderPath("Programs")) "jbn-player.lnk"
    $sh  = (New-Object -ComObject WScript.Shell).CreateShortcut($lnk)
    $sh.TargetPath = Join-Path $dest "jbn-player.exe"
    $sh.WorkingDirectory = $dest
    $sh.Save()

    Write-Host "Installed in $dest. Starting jbn-player (look for the tray icon)."
    Start-Process (Join-Path $dest "jbn-player.exe") -WorkingDirectory $dest
} finally {
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
