param(
  [string]$ControlUrl = "",
  [string]$ControlToken = "",
  [string]$ReportPath = "dist/verification/edge-rehearsal-latest.json",
  [switch]$RegisterRemote
)

$ErrorActionPreference = "Stop"

$repositoryRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$serverRoot = Join-Path $repositoryRoot "server"
$reportFullPath = if ([System.IO.Path]::IsPathRooted($ReportPath)) {
  $ReportPath
} else {
  Join-Path $repositoryRoot $ReportPath
}
$reportDirectory = Split-Path -Parent $reportFullPath
$binaryName = if ($IsWindows -or $env:OS -eq "Windows_NT") { "edge-rehearsal.exe" } else { "edge-rehearsal" }
$packagedBinary = Join-Path $repositoryRoot "bin\$binaryName"

if (-not (Test-Path $reportDirectory)) {
  New-Item -ItemType Directory -Path $reportDirectory -Force | Out-Null
}

$arguments = @()
if (-not [string]::IsNullOrWhiteSpace($ControlUrl)) {
  $arguments += @("-control-url", $ControlUrl)
}
if (-not [string]::IsNullOrWhiteSpace($ControlToken)) {
  $arguments += @("-control-token", $ControlToken)
}
if ($RegisterRemote) {
  $arguments += "-register-remote"
}

if (Test-Path $packagedBinary) {
  $output = & $packagedBinary @arguments
  $output | Set-Content -Path $reportFullPath -Encoding UTF8
  Write-Output $output
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }
  return
}

$previousGoCache = $env:GOCACHE
Push-Location $serverRoot
try {
  if ([string]::IsNullOrWhiteSpace($env:GOCACHE)) {
    $env:GOCACHE = Join-Path $repositoryRoot ".gocache-test"
  }
  if (-not (Test-Path $env:GOCACHE)) {
    New-Item -ItemType Directory -Path $env:GOCACHE -Force | Out-Null
  }
  $goArguments = @("run", "./cmd/edge-rehearsal") + $arguments
  $output = & go @goArguments
  $output | Set-Content -Path $reportFullPath -Encoding UTF8
  Write-Output $output
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }
} finally {
  $env:GOCACHE = $previousGoCache
  Pop-Location
}
