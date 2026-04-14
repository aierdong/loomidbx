# 编译 Go 共享库到 backend/build（Windows）
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$outDir = Join-Path $root "backend\build"
if (-not (Test-Path $outDir)) {
    New-Item -ItemType Directory -Path $outDir | Out-Null
}
& go build -buildmode=c-shared -o (Join-Path $outDir "libldb.dll") ./backend/cmd
Write-Host "Built: $(Join-Path $outDir 'libldb.dll')"
