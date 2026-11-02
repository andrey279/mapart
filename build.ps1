param([switch]$RefreshManifest)
$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    if ($RefreshManifest -or !(Test-Path -LiteralPath 'rsrc_windows_amd64.syso')) {
        if (Get-Command windres -ErrorAction SilentlyContinue) {
            & windres -i app.rc -o rsrc_windows_amd64.syso -O coff
        } else {
            & go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -manifest app.manifest -o rsrc_windows_amd64.syso
        }
        if ($LASTEXITCODE -ne 0) { throw 'Windows resource build failed' }
    }
    & go build -o mapart.exe .
    if ($LASTEXITCODE -ne 0) { throw 'CLI build failed' }
    & go build -ldflags '-H windowsgui' -o mapart-gui.exe .
    if ($LASTEXITCODE -ne 0) { throw 'GUI build failed' }
    Write-Host 'Built mapart.exe and mapart-gui.exe'
} finally {
    Pop-Location
}
