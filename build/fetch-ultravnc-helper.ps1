param(
  [string]$Version = "1640",
  [string]$OutDir = "build"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $OutDir)) {
  New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
}

$zipPath = Join-Path $env:TEMP ("UltraVNC_" + $Version + ".zip")
$extractDir = Join-Path $env:TEMP ("UltraVNC_" + $Version + "_extract")
$zipUrl = "https://uvnc.eu/download/$Version/UltraVNC_${Version}.zip"
$sourceExe = Join-Path $extractDir "x64\\winvnc.exe"
$targetExe = Join-Path $OutDir "acremote-helper.exe"

Write-Host "Downloading UltraVNC helper from $zipUrl ..."
Invoke-WebRequest -Uri $zipUrl -OutFile $zipPath

if (Test-Path $extractDir) {
  Remove-Item -Path $extractDir -Recurse -Force
}
Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force

if (-not (Test-Path $sourceExe)) {
  throw "winvnc.exe not found at $sourceExe"
}

Copy-Item -Path $sourceExe -Destination $targetExe -Force
Write-Host "Remote helper written: $targetExe"
