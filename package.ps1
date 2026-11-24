# Builds the release asset dist/jbn-player-windows-x64.zip (exe only; libmpv is
# downloaded by install.ps1). Upload it to a GitHub Release, e.g.:
#   gh release create v0.1.0 dist/jbn-player-windows-x64.zip
$ErrorActionPreference = "Stop"
./build.ps1
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
New-Item -ItemType Directory -Force dist | Out-Null
Compress-Archive -Force -Path bin/jbn-player.exe, LICENSE -DestinationPath dist/jbn-player-windows-x64.zip
Write-Host "OK -> dist/jbn-player-windows-x64.zip"
