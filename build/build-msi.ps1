param(
  [string]$Version = "",
  [string]$BuildDir = "build",
  [string]$SourceDir = ".",
  [string]$OutDir = "build"
)

$ErrorActionPreference = "Stop"

function Get-AgentVersionFromTemplate([string]$path) {
  $lines = Get-Content $path
  foreach ($l in $lines) {
    if ($l -match '^\s*version:\s*\"?([0-9]+\.[0-9]+\.[0-9]+)\"?\s*$') { return $Matches[1] }
  }
  throw "Could not parse agent version from $path (expected x.y.z)"
}

if (-not $Version) {
  $Version = Get-AgentVersionFromTemplate (Join-Path $SourceDir "configs\\config.yaml.template")
}

New-Item -ItemType Directory -Force $OutDir | Out-Null

$wxs = Join-Path $SourceDir "installer\\wix\\AppCenterAgent.wxs"
if (-not (Test-Path $wxs)) { throw "Missing wix source: $wxs" }

if (-not (Get-Command candle.exe -ErrorAction SilentlyContinue)) {
  throw "candle.exe not found in PATH. Install WiX Toolset v3.x (e.g. choco install wixtoolset)."
}
if (-not (Get-Command light.exe -ErrorAction SilentlyContinue)) {
  throw "light.exe not found in PATH. Install WiX Toolset v3.x (e.g. choco install wixtoolset)."
}

$obj = Join-Path $OutDir "AppCenterAgent.wixobj"
$msi = Join-Path $OutDir ("AppCenterAgent-" + $Version + ".msi")

Write-Host "Building MSI version=$Version ..."

& candle.exe `
  "-dProductVersion=$Version" `
  "-dBuildDir=$BuildDir" `
  "-dSourceDir=$SourceDir" `
  "-out" $obj `
  $wxs
if ($LASTEXITCODE -ne 0) {
  throw "candle.exe failed with exit code $LASTEXITCODE"
}

& light.exe `
  "-out" $msi `
  $obj
if ($LASTEXITCODE -ne 0) {
  throw "light.exe failed with exit code $LASTEXITCODE"
}

if (-not (Test-Path $msi)) {
  throw "MSI output not found: $msi"
}

Write-Host "MSI written: $msi"
