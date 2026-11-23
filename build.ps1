# Builds jbn-player.exe (windowsgui, stripped) into bin/ and makes sure
# libmpv-2.dll sits next to it. Get the DLL from the shinchiro mpv-winbuild
# "mpv-dev-x86_64" release and drop libmpv-2.dll into bin/.
$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force bin | Out-Null
go test ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go build -ldflags "-H=windowsgui -s -w" -o bin/jbn-player.exe ./cmd/jbn-player
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
if (-not (Test-Path bin/libmpv-2.dll)) { Write-Warning "bin/libmpv-2.dll is missing: the player will not start." }
if (Test-Path .env) { Copy-Item .env bin/.env -Force }
Write-Host "OK -> bin/jbn-player.exe"
