param(
  [string]$Version = "v0.6.4-alpha",
  [switch]$SkipWeb
)

$ErrorActionPreference = "Stop"

$repositoryRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$serverRoot = Join-Path $repositoryRoot "server"
$cliRoot = Join-Path $repositoryRoot "cli"
$webRoot = Join-Path $serverRoot "web"
$webDistRoot = Join-Path $webRoot "dist"
$deployRoot = Join-Path $repositoryRoot "deploy\server"
$distRoot = Join-Path $repositoryRoot "dist"
$goCacheRoot = Join-Path $repositoryRoot ".gocache-release"
$releaseVersion = $Version.Trim()

if ([string]::IsNullOrWhiteSpace($releaseVersion)) {
  throw "Version 不能为空"
}

$normalizedVersion = $releaseVersion.TrimStart("v")

function Invoke-WebBuild {
  if ($SkipWeb) {
    Write-Host "跳过 Dashboard Web 构建"
    return
  }
  Write-Host "构建 Dashboard Web"
  $npmCommand = Get-Command npm -ErrorAction SilentlyContinue
  if ($npmCommand) {
    Push-Location $webRoot
    try {
      npm run build
    } finally {
      Pop-Location
    }
    return
  }
  $bundledNode = Join-Path $env:USERPROFILE ".cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe"
  if (-not (Test-Path $bundledNode)) {
    throw "未找到 npm，也未找到 bundled Node：$bundledNode"
  }
  Push-Location $webRoot
  try {
    & $bundledNode "node_modules\vue-tsc\bin\vue-tsc.js" "--noEmit"
    & $bundledNode "node_modules\vite\bin\vite.js" "build"
  } finally {
    Pop-Location
  }
}

function New-ServerArchive {
  param(
    [string]$GOOS,
    [string]$GOARCH,
    [string]$Archive,
    [string]$Exe
  )

  $env:GOOS = $GOOS
  $env:GOARCH = $GOARCH
  $env:CGO_ENABLED = "0"
  $packageName = "nextunnel-server-$GOOS-$GOARCH"
  $packageDir = Join-Path $distRoot $packageName
  $binDir = Join-Path $packageDir "bin"
  $dashboardDir = Join-Path $packageDir "web\dashboard"
  if (Test-Path $packageDir) {
    Remove-Item -LiteralPath $packageDir -Recurse -Force
  }
  New-Item -ItemType Directory -Path $binDir -Force | Out-Null
  New-Item -ItemType Directory -Path $dashboardDir -Force | Out-Null

  Push-Location $serverRoot
  try {
    go build -trimpath -ldflags="-s -w" -o (Join-Path $binDir "control-plane$Exe") ./cmd/control-plane
    go build -trimpath -ldflags="-s -w" -o (Join-Path $binDir "relay-server$Exe") ./cmd/relay
    go build -trimpath -ldflags="-s -w" -o (Join-Path $binDir "nat-detector$Exe") ./cmd/nat-detector
    go build -trimpath -ldflags="-s -w" -o (Join-Path $binDir "dashboard$Exe") ./cmd/dashboard
    go build -trimpath -ldflags="-s -w" -o (Join-Path $binDir "edge-rehearsal$Exe") ./cmd/edge-rehearsal
    go build -trimpath -ldflags="-s -w" -o (Join-Path $binDir "ebpf-verify$Exe") ./cmd/ebpf-verify
  } finally {
    Pop-Location
  }
  Push-Location $cliRoot
  try {
    go build -trimpath -ldflags "-s -w -X main.version=$normalizedVersion" -o (Join-Path $binDir "nextunnel$Exe") .
  } finally {
    Pop-Location
  }

  Copy-Item -Path (Join-Path $webDistRoot "*") -Destination $dashboardDir -Recurse -Force
  Copy-Item -LiteralPath (Join-Path $deployRoot ".env.example") -Destination (Join-Path $packageDir ".env.example") -Force
  Copy-Item -LiteralPath (Join-Path $deployRoot "README.md") -Destination (Join-Path $packageDir "README.md") -Force
  $packageDeployDir = Join-Path $packageDir "deploy\server"
  New-Item -ItemType Directory -Path $packageDeployDir -Force | Out-Null
  Copy-Item -LiteralPath (Join-Path $deployRoot "install.sh") -Destination (Join-Path $packageDeployDir "install.sh") -Force
  Copy-Item -LiteralPath (Join-Path $deployRoot "install.ps1") -Destination (Join-Path $packageDeployDir "install.ps1") -Force
  $packageScriptsDir = Join-Path $packageDir "scripts"
  New-Item -ItemType Directory -Path $packageScriptsDir -Force | Out-Null
  Copy-Item -LiteralPath (Join-Path $repositoryRoot "scripts\verify-dashboard.ps1") -Destination (Join-Path $packageScriptsDir "verify-dashboard.ps1") -Force
  Copy-Item -LiteralPath (Join-Path $repositoryRoot "scripts\verify-dashboard-ssh.ps1") -Destination (Join-Path $packageScriptsDir "verify-dashboard-ssh.ps1") -Force
  Copy-Item -LiteralPath (Join-Path $repositoryRoot "scripts\verify-edge-rehearsal.ps1") -Destination (Join-Path $packageScriptsDir "verify-edge-rehearsal.ps1") -Force
  Copy-Item -LiteralPath (Join-Path $repositoryRoot "scripts\verify-ebpf-linux.sh") -Destination (Join-Path $packageScriptsDir "verify-ebpf-linux.sh") -Force
  Copy-Item -LiteralPath (Join-Path $repositoryRoot "scripts\verify-scripts-static.ps1") -Destination (Join-Path $packageScriptsDir "verify-scripts-static.ps1") -Force
  Copy-Item -LiteralPath (Join-Path $repositoryRoot "server\internal\ebpf\xdp_forwarder.c") -Destination (Join-Path $packageDir "xdp_forwarder.c") -Force
  @(
    "NexTunnel server package",
    "Version: $releaseVersion",
    "Target: $GOOS-$GOARCH",
    "Binaries:",
    "  bin/control-plane$Exe",
    "  bin/relay-server$Exe",
    "  bin/nat-detector$Exe",
    "  bin/dashboard$Exe",
    "  bin/edge-rehearsal$Exe",
    "  bin/ebpf-verify$Exe",
    "  bin/nextunnel$Exe",
    "Web:",
    "  web/dashboard",
    "Deploy:",
    "  deploy/server/install.sh",
    "  deploy/server/install.ps1",
    "Verification:",
    "  scripts/verify-dashboard.ps1",
    "  scripts/verify-dashboard-ssh.ps1",
    "  scripts/verify-edge-rehearsal.ps1",
    "  scripts/verify-ebpf-linux.sh",
    "  scripts/verify-scripts-static.ps1",
    "  xdp_forwarder.c"
  ) | Set-Content -Path (Join-Path $packageDir "MANIFEST.txt") -Encoding UTF8

  if ($Archive -eq "tar.gz") {
    $archivePath = Join-Path $distRoot "$packageName.tar.gz"
    if (Test-Path $archivePath) { Remove-Item -LiteralPath $archivePath -Force }
    tar -czf $archivePath -C $distRoot $packageName
  } else {
    $archivePath = Join-Path $distRoot "$packageName.zip"
    if (Test-Path $archivePath) { Remove-Item -LiteralPath $archivePath -Force }
    Compress-Archive -Path $packageDir -DestinationPath $archivePath -Force
  }
  $hash = Get-FileHash -Algorithm SHA256 -Path $archivePath
  "$($hash.Hash.ToLowerInvariant())  $(Split-Path -Leaf $archivePath)" | Set-Content -Path "$archivePath.sha256" -Encoding ASCII
  Write-Host "服务端发布包已生成：$archivePath"
}

if (-not (Test-Path $distRoot)) {
  New-Item -ItemType Directory -Path $distRoot | Out-Null
}
if (-not (Test-Path $goCacheRoot)) {
  New-Item -ItemType Directory -Path $goCacheRoot | Out-Null
}

Invoke-WebBuild

$previousGoCache = $env:GOCACHE
$previousGoOS = $env:GOOS
$previousGoArch = $env:GOARCH
$previousCgoEnabled = $env:CGO_ENABLED
$env:GOCACHE = $goCacheRoot

try {
  New-ServerArchive -GOOS "linux" -GOARCH "amd64" -Archive "tar.gz" -Exe ""
  New-ServerArchive -GOOS "linux" -GOARCH "arm64" -Archive "tar.gz" -Exe ""
  New-ServerArchive -GOOS "windows" -GOARCH "amd64" -Archive "zip" -Exe ".exe"
} finally {
  $env:GOCACHE = $previousGoCache
  $env:GOOS = $previousGoOS
  $env:GOARCH = $previousGoArch
  $env:CGO_ENABLED = $previousCgoEnabled
}
