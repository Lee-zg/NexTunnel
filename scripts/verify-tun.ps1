param(
  [string]$Interface = "nextunnel0",
  [string]$VirtualIP = "10.7.0.10",
  [string]$PeerIP = "10.7.0.1",
  [string]$Subnet = "10.7.0.0/24",
  [string]$Gateway = "10.7.0.1",
  [string]$Route = "10.7.0.0/24",
  [string]$ReportPath = "dist/verification/tun-local-latest.json",
  [switch]$SkipRouteApply
)

$ErrorActionPreference = "Stop"

$repositoryRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$desktopRoot = Join-Path $repositoryRoot "desktop"
$reportFullPath = if ([System.IO.Path]::IsPathRooted($ReportPath)) {
  $ReportPath
} else {
  Join-Path $repositoryRoot $ReportPath
}
$reportDirectory = Split-Path -Parent $reportFullPath

if (-not (Test-Path $reportDirectory)) {
  New-Item -ItemType Directory -Path $reportDirectory -Force | Out-Null
}

$arguments = @(
  "run", "./cmd/tun-verify",
  "-interface", $Interface,
  "-virtual-ip", $VirtualIP,
  "-peer-ip", $PeerIP,
  "-subnet", $Subnet,
  "-gateway", $Gateway,
  "-route", $Route
)
if ($SkipRouteApply) {
  $arguments += "-skip-route-apply"
}

$previousGoCache = $env:GOCACHE
Push-Location $desktopRoot
try {
  if ([string]::IsNullOrWhiteSpace($env:GOCACHE)) {
    $env:GOCACHE = Join-Path $repositoryRoot ".gocache-test-desktop"
  }
  if (-not (Test-Path $env:GOCACHE)) {
    New-Item -ItemType Directory -Path $env:GOCACHE -Force | Out-Null
  }
  $output = & go @arguments
  $output | Set-Content -Path $reportFullPath -Encoding UTF8
  Write-Output $output
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }
} finally {
  $env:GOCACHE = $previousGoCache
  Pop-Location
}
