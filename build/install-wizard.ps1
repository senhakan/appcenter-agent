param(
  [string]$MsiPath = "",
  [string]$ServerUrl = "",
  [string]$SecretKey = "",
  [switch]$Silent
)

$ErrorActionPreference = "Stop"

function Resolve-MsiPath {
  param([string]$InputPath)
  if ($InputPath -and (Test-Path $InputPath)) {
    return (Resolve-Path $InputPath).Path
  }

  $buildDir = Split-Path -Parent $PSCommandPath
  $candidate = Get-ChildItem -Path $buildDir -Filter "AppCenterAgent-*.msi" -File |
    Sort-Object LastWriteTime -Descending |
    Select-Object -First 1

  if (-not $candidate) {
    throw "MSI not found. Use -MsiPath to pass installer path."
  }
  return $candidate.FullName
}

function Read-SecretPlain {
  param([string]$Prompt)
  $secure = Read-Host -Prompt $Prompt -AsSecureString
  $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
  try {
    return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
  } finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
  }
}

$msi = Resolve-MsiPath -InputPath $MsiPath
Write-Host "MSI: $msi"

$serverUrl = $ServerUrl
if (-not $serverUrl) {
  $serverUrl = Read-Host -Prompt "AppCenter Server URL (example: http://10.6.100.170:8000)"
}
if (-not $serverUrl) {
  throw "Server URL is required."
}

$secretKey = $SecretKey
if (-not $secretKey) {
  $secretKey = Read-SecretPlain -Prompt "Agent Secret Key (optional; empty to auto-register)"
}

$args = @(
  "/i", "`"$msi`"",
  "SERVER_URL=$serverUrl"
)

if ($secretKey) {
  $args += "SECRET_KEY=$secretKey"
}

if ($Silent) {
  $args += "/qn"
} else {
  $args += "/qb"
}
$args += "/norestart"

Write-Host "Starting MSI install..."
$p = Start-Process -FilePath "msiexec.exe" -ArgumentList $args -Wait -PassThru
Write-Host "MSI exit code: $($p.ExitCode)"
exit $p.ExitCode
