param(
  [string]$Version = "v0.6.4-alpha"
)

$ErrorActionPreference = "Stop"

$repositoryRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$cliRoot = Join-Path $repositoryRoot "cli"
$distRoot = Join-Path $repositoryRoot "dist"
$goCacheRoot = Join-Path $repositoryRoot ".gocache-release"
$releaseVersion = $Version.Trim()

if ([string]::IsNullOrWhiteSpace($releaseVersion)) {
  throw "Version 不能为空"
}

$normalizedVersion = $releaseVersion.TrimStart("v")
$targets = @(
  @{ GOOS = "linux"; GOARCH = "amd64"; Archive = "tar.gz"; Exe = "" },
  @{ GOOS = "linux"; GOARCH = "arm64"; Archive = "tar.gz"; Exe = "" },
  @{ GOOS = "windows"; GOARCH = "amd64"; Archive = "zip"; Exe = ".exe" }
)

if (-not (Test-Path $distRoot)) {
  New-Item -ItemType Directory -Path $distRoot | Out-Null
}
if (-not (Test-Path $goCacheRoot)) {
  New-Item -ItemType Directory -Path $goCacheRoot | Out-Null
}

$previousGoCache = $env:GOCACHE
$previousGoOS = $env:GOOS
$previousGoArch = $env:GOARCH
$previousCgoEnabled = $env:CGO_ENABLED
$env:GOCACHE = $goCacheRoot

try {
  foreach ($target in $targets) {
    $env:GOOS = $target.GOOS
    $env:GOARCH = $target.GOARCH
    $env:CGO_ENABLED = "0"
    $packageName = "nextunnel-cli-$releaseVersion-$($target.GOOS)-$($target.GOARCH)"
    $packageDir = Join-Path $distRoot $packageName
    if (Test-Path $packageDir) {
      Remove-Item -LiteralPath $packageDir -Recurse -Force
    }
    New-Item -ItemType Directory -Path $packageDir | Out-Null
    $binaryName = "nextunnel$($target.Exe)"
    Push-Location $cliRoot
    try {
      go build -trimpath -ldflags "-s -w -X main.version=$normalizedVersion" -o (Join-Path $packageDir $binaryName) .
    } finally {
      Pop-Location
    }
    @(
      "NexTunnel CLI package",
      "Version: $releaseVersion",
      "ApplicationVersion: $normalizedVersion",
      "Target: $($target.GOOS)/$($target.GOARCH)",
      "Binary: $binaryName"
    ) | Set-Content -Path (Join-Path $packageDir "MANIFEST.txt") -Encoding UTF8

    if ($target.Archive -eq "tar.gz") {
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
    Write-Host "CLI 发布包已生成：$archivePath"
  }
} finally {
  $env:GOCACHE = $previousGoCache
  $env:GOOS = $previousGoOS
  $env:GOARCH = $previousGoArch
  $env:CGO_ENABLED = $previousCgoEnabled
}
