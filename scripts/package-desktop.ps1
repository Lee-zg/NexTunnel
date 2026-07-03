param(
  [string]$Version = "v0.6.4-alpha",
  [string]$Platform = "windows/amd64",
  [ValidateSet("bundled", "download", "manual")]
  [string]$WintunMode = "bundled",
  [string]$WintunDllPath = "",
  [string]$WintunDownloadUrl = "https://www.wintun.net/builds/wintun-0.14.1.zip",
  [string]$WintunSha256 = "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51",
  [switch]$SkipFrontend,
  [switch]$SkipZip
)

$ErrorActionPreference = "Stop"

$repositoryRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$desktopRoot = Join-Path $repositoryRoot "desktop"
$desktopFrontendRoot = Join-Path $desktopRoot "frontend"
$installerRoot = Join-Path $repositoryRoot "installer"
$installerFrontendRoot = Join-Path $installerRoot "frontend"
$installerPayloadRoot = Join-Path $installerRoot "payload"
$distRoot = Join-Path $repositoryRoot "dist"
$releaseVersion = $Version.Trim()
$supportedDesktopPlatform = "windows/amd64"
$wintunMachineAmd64 = 0x8664
$wintunArchiveRelativeDllPath = "wintun\bin\amd64\wintun.dll"
$goCacheRoot = Join-Path $repositoryRoot ".gocache-release"
$wintunDownloadRoot = Join-Path $repositoryRoot ".tmp\wintun-release"
$preparedWintunDllPath = ""
$payloadIncludesWintun = $false

if ([string]::IsNullOrWhiteSpace($releaseVersion)) {
  throw "Version 不能为空"
}
if ($releaseVersion -notmatch '^v?[0-9A-Za-z][0-9A-Za-z.\-+]*$') {
  throw "Version 包含非法字符：$releaseVersion"
}
if ($Platform -ne $supportedDesktopPlatform) {
  throw "当前桌面端发布脚本仅支持 $supportedDesktopPlatform"
}
if (($WintunMode -eq "bundled" -or $WintunMode -eq "download") -and [string]::IsNullOrWhiteSpace($WintunSha256)) {
  throw "WintunMode=$WintunMode 时必须提供 WintunSha256，避免发布不可校验的 DLL 来源"
}

$normalizedVersion = $releaseVersion.TrimStart("v")
$windowsResourceVersion = "$normalizedVersion.0"
$targetName = "nextunnel-$releaseVersion-windows-amd64"
$targetDirectory = Join-Path $distRoot $targetName
$binaryOutputName = "$targetName.exe"
$binarySource = Join-Path $desktopRoot "build\bin\$binaryOutputName"
$binaryTarget = Join-Path $targetDirectory $binaryOutputName
$portableArchivePath = Join-Path $distRoot "$targetName.zip"
$installerPayloadZipName = "nextunnel-payload.zip"
$installerPayloadZipPath = Join-Path $installerPayloadRoot $installerPayloadZipName
$installerPayloadManifestPath = Join-Path $installerPayloadRoot "manifest.json"
$installerOutputName = "NexTunnelInstaller.exe"
$installerSource = Join-Path $installerRoot "build\bin\$installerOutputName"
$installerTarget = Join-Path $distRoot "$targetName-installer.exe"
$originalInstallerManifest = $null
$hadInstallerManifest = $false
$hadInstallerPayloadZip = $false

function New-DirectoryIfMissing {
  param([string]$Path)
  if (-not (Test-Path $Path)) {
    New-Item -ItemType Directory -Path $Path | Out-Null
  }
}

function Assert-UnderDirectory {
  param(
    [string]$ChildPath,
    [string]$ParentPath
  )
  $resolvedParent = [System.IO.Path]::GetFullPath($ParentPath).TrimEnd(
    [System.IO.Path]::DirectorySeparatorChar,
    [System.IO.Path]::AltDirectorySeparatorChar
  )
  $resolvedChild = [System.IO.Path]::GetFullPath($ChildPath)
  $repoPathPrefix = $resolvedParent + [System.IO.Path]::DirectorySeparatorChar
  if ($resolvedChild -ne $resolvedParent -and -not $resolvedChild.StartsWith($repoPathPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "目标路径不在允许目录内：$resolvedChild"
  }
}

function New-Sha256File {
  param([string]$Path)
  if (-not (Test-Path $Path)) {
    throw "无法生成 SHA256，文件不存在：$Path"
  }
  $hash = Get-FileHash -Algorithm SHA256 -Path $Path
  $checksumPath = "$Path.sha256"
  "$($hash.Hash.ToLowerInvariant())  $(Split-Path -Leaf $Path)" | Set-Content -Path $checksumPath -Encoding ASCII
  return $checksumPath
}

function Assert-NativeCommandSucceeded {
  param([string]$Action)
  if ($LASTEXITCODE -ne 0) {
    throw "$Action 失败，退出码：$LASTEXITCODE"
  }
}

function Assert-Sha256File {
  param(
    [string]$Path,
    [string]$ExpectedSha256
  )
  if ([string]::IsNullOrWhiteSpace($ExpectedSha256)) {
    throw "缺少 SHA256，无法校验文件：$Path"
  }
  $actualHash = (Get-FileHash -Algorithm SHA256 -Path $Path).Hash.ToLowerInvariant()
  $expectedHash = $ExpectedSha256.Trim().ToLowerInvariant()
  if ($actualHash -ne $expectedHash) {
    throw "SHA256 校验失败：$Path expected=$expectedHash actual=$actualHash"
  }
}

function Get-WintunDllCandidatePath {
  $candidatePaths = [System.Collections.Generic.List[string]]::new()
  if (-not [string]::IsNullOrWhiteSpace($WintunDllPath)) {
    $candidatePaths.Add($WintunDllPath) | Out-Null
  }
  if (-not [string]::IsNullOrWhiteSpace($env:NEXTUNNEL_WINTUN_DLL)) {
    $candidatePaths.Add($env:NEXTUNNEL_WINTUN_DLL) | Out-Null
  }
  $candidatePaths.Add((Join-Path $desktopRoot "wintun.dll")) | Out-Null
  $candidatePaths.Add((Join-Path $repositoryRoot "wintun.dll")) | Out-Null
  if (-not [string]::IsNullOrWhiteSpace($env:ProgramFiles)) {
    $candidatePaths.Add((Join-Path $env:ProgramFiles "WireGuard\wintun.dll")) | Out-Null
    $candidatePaths.Add((Join-Path $env:ProgramFiles "Wintun\bin\amd64\wintun.dll")) | Out-Null
  }
  $programFilesX86 = ${env:ProgramFiles(x86)}
  if (-not [string]::IsNullOrWhiteSpace($programFilesX86)) {
    $candidatePaths.Add((Join-Path $programFilesX86 "Wintun\bin\amd64\wintun.dll")) | Out-Null
  }
  foreach ($candidatePath in $candidatePaths) {
    if (Test-Path $candidatePath) {
      return (Resolve-Path $candidatePath).Path
    }
  }
  return ""
}

function Get-PreparedWintunDllPath {
  if (-not [string]::IsNullOrWhiteSpace($preparedWintunDllPath) -and (Test-Path $preparedWintunDllPath)) {
    return $preparedWintunDllPath
  }
  return Get-WintunDllCandidatePath
}

function Get-PEDllMachine {
  param([string]$Path)
  $stream = [System.IO.File]::OpenRead($Path)
  try {
    if ($stream.Length -lt 0x40) {
      throw "DLL is too small: $Path"
    }
    $reader = [System.IO.BinaryReader]::new($stream)
    try {
      $stream.Seek(0x3C, [System.IO.SeekOrigin]::Begin) | Out-Null
      $peOffset = $reader.ReadInt32()
      if ($peOffset -lt 0 -or ($peOffset + 6) -gt $stream.Length) {
        throw "invalid PE header offset: $Path"
      }
      $stream.Seek($peOffset, [System.IO.SeekOrigin]::Begin) | Out-Null
      $signature = $reader.ReadUInt32()
      if ($signature -ne 0x00004550) {
        throw "invalid PE signature: $Path"
      }
      return $reader.ReadUInt16()
    } finally {
      $reader.Close()
    }
  } finally {
    $stream.Close()
  }
}

function Get-ExpectedWintunDllMachine {
  # PE Machine 值用于阻止错误架构的 wintun.dll 被打进发布包。
  if ($Platform -eq "windows/amd64") {
    return $wintunMachineAmd64
  }
  throw "当前平台不支持 Wintun 架构校验：$Platform"
}

function Assert-WintunDllArchitecture {
  param([string]$Path)
  $expectedMachine = Get-ExpectedWintunDllMachine
  $actualMachine = Get-PEDllMachine -Path $Path
  if ($actualMachine -ne $expectedMachine) {
    throw ("wintun.dll 架构不匹配：path={0} expected=0x{1:X4} actual=0x{2:X4}" -f $Path, $expectedMachine, $actualMachine)
  }
}

function Save-OfficialWintunDll {
  if ($WintunMode -eq "manual") {
    return
  }

  $existingWintunPath = Get-WintunDllCandidatePath
  if (-not [string]::IsNullOrWhiteSpace($existingWintunPath)) {
    Assert-WintunDllArchitecture -Path $existingWintunPath
    $script:preparedWintunDllPath = $existingWintunPath
    Write-Host "已使用本地官方 Wintun DLL：$existingWintunPath"
    return
  }

  if ($WintunDownloadUrl -notmatch '^https://') {
    throw "WintunDownloadUrl 必须使用 HTTPS：$WintunDownloadUrl"
  }
  New-DirectoryIfMissing -Path $wintunDownloadRoot

  $wintunZipPath = Join-Path $wintunDownloadRoot "wintun.zip"
  $wintunExtractRoot = Join-Path $wintunDownloadRoot "extract"
  if (Test-Path $wintunExtractRoot) {
    Remove-Item -LiteralPath $wintunExtractRoot -Recurse -Force
  }

  Write-Host "下载官方 Wintun 包：$WintunDownloadUrl"
  Invoke-WebRequest -Uri $WintunDownloadUrl -OutFile $wintunZipPath
  Assert-Sha256File -Path $wintunZipPath -ExpectedSha256 $WintunSha256

  Expand-Archive -Path $wintunZipPath -DestinationPath $wintunExtractRoot -Force
  $extractedDllPath = Join-Path $wintunExtractRoot $wintunArchiveRelativeDllPath
  if (-not (Test-Path $extractedDllPath)) {
    throw "官方 Wintun 包中未找到预期 DLL：$wintunArchiveRelativeDllPath"
  }
  Assert-WintunDllArchitecture -Path $extractedDllPath
  $script:preparedWintunDllPath = $extractedDllPath
  Write-Host "已准备官方 Wintun DLL：$extractedDllPath"
}

function Invoke-FrontendBuild {
  param(
    [string]$Name,
    [string]$FrontendRoot
  )
  if ($SkipFrontend) {
    Write-Host "跳过 $Name 前端构建"
    return
  }

  Write-Host "构建 $Name 前端"
  $npmCommand = Get-Command npm -ErrorAction SilentlyContinue
  $pnpmCommand = Get-Command pnpm -ErrorAction SilentlyContinue
  $usesNpmLock = Test-Path (Join-Path $FrontendRoot "package-lock.json")
  # 桌面前端仍以 package-lock.json 为准；只有纯 pnpm 项目才走 pnpm 锁文件。
  $usesPnpmLock = (-not $usesNpmLock) -and (Test-Path (Join-Path $FrontendRoot "pnpm-lock.yaml"))
  Push-Location $FrontendRoot
  try {
    if ($usesPnpmLock) {
      if (-not $pnpmCommand) {
        throw "$Name 前端使用 pnpm-lock.yaml，但当前环境未安装 pnpm。请先运行 corepack enable 并安装 pnpm。"
      }
      $previousPath = $env:Path
      $previousCI = $env:CI
      $bundledNodeDirectory = Join-Path $env:USERPROFILE ".cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin"
      if (Test-Path $bundledNodeDirectory) {
        # pnpm 的 postinstall 会直接调用 node，受限环境下需要显式补齐 PATH。
        $env:Path = "$bundledNodeDirectory;$env:Path"
      }
      if ([string]::IsNullOrWhiteSpace($env:CI)) {
        $env:CI = "true"
      }
      try {
        # 安装器前端使用 pnpm 锁文件，发布构建必须按锁文件安装，避免 CI 缺依赖或依赖漂移。
        pnpm install --frozen-lockfile --config.confirmModulesPurge=false
        Assert-NativeCommandSucceeded -Action "$Name 前端 pnpm install"
        pnpm run build
        Assert-NativeCommandSucceeded -Action "$Name 前端 pnpm build"
      } finally {
        $env:Path = $previousPath
        if ([string]::IsNullOrWhiteSpace($previousCI)) {
          Remove-Item Env:CI -ErrorAction SilentlyContinue
        } else {
          $env:CI = $previousCI
        }
      }
      return
    }

    if ($npmCommand) {
      npm run build
      Assert-NativeCommandSucceeded -Action "$Name 前端 npm build"
      return
    }
  } finally {
    Pop-Location
  }

  $bundledNode = Join-Path $env:USERPROFILE ".cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe"
  if (-not (Test-Path $bundledNode)) {
    throw "未找到 npm/pnpm，也未找到 bundled Node：$bundledNode"
  }

  Push-Location $FrontendRoot
  try {
    & $bundledNode "node_modules\vue-tsc\bin\vue-tsc.js" "--noEmit"
    Assert-NativeCommandSucceeded -Action "$Name 前端 vue-tsc"
    & $bundledNode "node_modules\vite\bin\vite.js" "build"
    Assert-NativeCommandSucceeded -Action "$Name 前端 vite build"
  } finally {
    Pop-Location
  }
}

function Reset-InstallerFrontendDist {
  $installerFrontendDist = Join-Path $installerFrontendRoot "dist"
  Assert-UnderDirectory -ChildPath $installerFrontendDist -ParentPath $installerFrontendRoot
  New-DirectoryIfMissing -Path $installerFrontendDist
  Get-ChildItem -LiteralPath $installerFrontendDist -Force | Where-Object {
    $_.Name -ne ".gitkeep"
  } | Remove-Item -Recurse -Force
  New-Item -ItemType File -Path (Join-Path $installerFrontendDist ".gitkeep") -Force | Out-Null
}

function Assert-InstallerFrontendBuilt {
  $installerFrontendIndex = Join-Path $installerFrontendRoot "dist\index.html"
  if (-not (Test-Path $installerFrontendIndex)) {
    throw "安装器前端产物缺失：$installerFrontendIndex。请移除 -SkipFrontend 或先在 installer/frontend 运行前端构建。"
  }
}

function Invoke-DesktopWailsBuild {
  Write-Host "打包桌面端 $releaseVersion ($Platform)"
  New-DirectoryIfMissing -Path $goCacheRoot
  Push-Location $desktopRoot
  try {
    $previousGoCache = $env:GOCACHE
    $env:GOCACHE = $goCacheRoot
    $buildArguments = @(
      "build",
      "-m",
      "-s",
      "-trimpath",
      "-platform", $Platform,
      "-webview2", "download",
      "-o", $binaryOutputName,
      "-ldflags", "-s -w -X main.AppVersion=$normalizedVersion"
    )
    & wails @buildArguments
    if ($LASTEXITCODE -ne 0) {
      throw "桌面端 Wails 构建失败，退出码：$LASTEXITCODE"
    }
  } finally {
    $env:GOCACHE = $previousGoCache
    Pop-Location
  }
}

function New-DesktopPayloadDirectory {
  if (-not (Test-Path $binarySource)) {
    throw "未找到 Wails 构建产物：$binarySource"
  }

  Assert-UnderDirectory -ChildPath $targetDirectory -ParentPath $distRoot
  if (Test-Path $targetDirectory) {
    Remove-Item -LiteralPath $targetDirectory -Recurse -Force
  }
  New-Item -ItemType Directory -Path $targetDirectory | Out-Null

  Copy-Item -LiteralPath $binarySource -Destination $binaryTarget
  $wintunSourcePath = Get-PreparedWintunDllPath
  $wintunManifestLine = "Wintun: missing; installer payload does not include wintun.dll"
  $script:payloadIncludesWintun = $false
  if (-not [string]::IsNullOrWhiteSpace($wintunSourcePath)) {
    Assert-WintunDllArchitecture -Path $wintunSourcePath
    Copy-Item -LiteralPath $wintunSourcePath -Destination (Join-Path $targetDirectory "wintun.dll") -Force
    $wintunManifestLine = "Wintun: bundled; architecture verified for $Platform"
    $script:payloadIncludesWintun = $true
  }
  $manifestPath = Join-Path $targetDirectory "MANIFEST.txt"
  @(
    "NexTunnel desktop package"
    "Version: $releaseVersion"
    "ApplicationVersion: $normalizedVersion"
    "WindowsResourceVersion: $windowsResourceVersion"
    "Target: $Platform"
    "Installer: custom-wails-payload"
    "Binary: $binaryOutputName"
    $wintunManifestLine
    "Signing: unsigned-alpha"
    "PrunedResources: true"
    "WebView2: installed by custom Wails installer"
  ) | Set-Content -Path $manifestPath -Encoding UTF8
}

function New-PortableArchive {
  if ($SkipZip) {
    Write-Host "跳过 Windows 便携 zip"
    return
  }
  if (Test-Path $portableArchivePath) {
    Remove-Item -LiteralPath $portableArchivePath -Force
  }
  Compress-Archive -Path (Join-Path $targetDirectory "*") -DestinationPath $portableArchivePath -Force
  $checksumPath = New-Sha256File -Path $portableArchivePath

  Write-Host "桌面端便携包已生成：$portableArchivePath"
  Write-Host "SHA256：$checksumPath"
}

function Set-InstallerPayload {
  New-DirectoryIfMissing -Path $installerPayloadRoot
  $script:hadInstallerManifest = Test-Path $installerPayloadManifestPath
  if ($script:hadInstallerManifest) {
    $script:originalInstallerManifest = Get-Content -Path $installerPayloadManifestPath -Raw
  }
  $script:hadInstallerPayloadZip = Test-Path $installerPayloadZipPath

  if (Test-Path $installerPayloadZipPath) {
    Remove-Item -LiteralPath $installerPayloadZipPath -Force
  }
  Compress-Archive -Path (Join-Path $targetDirectory "*") -DestinationPath $installerPayloadZipPath -Force
  $payloadHash = (Get-FileHash -Algorithm SHA256 -Path $installerPayloadZipPath).Hash.ToLowerInvariant()

  $manifest = [ordered]@{
    version = $releaseVersion
    target = $Platform
    payload_file = $installerPayloadZipName
    payload_sha256 = $payloadHash
    app_executable = $binaryOutputName
    required_space_mb = 512
    wintun_included = $payloadIncludesWintun
    webview2_bootstrapper = "embedded-by-wails"
    signing = "unsigned-alpha"
  }
  $manifest | ConvertTo-Json -Depth 4 | Set-Content -Path $installerPayloadManifestPath -Encoding UTF8
  Write-Host "安装器 payload 已生成：$installerPayloadZipPath"
}

function Restore-InstallerPayload {
  if ($hadInstallerManifest) {
    $originalInstallerManifest | Set-Content -Path $installerPayloadManifestPath -Encoding UTF8
  } elseif (Test-Path $installerPayloadManifestPath) {
    Remove-Item -LiteralPath $installerPayloadManifestPath -Force
  }
  if (-not $hadInstallerPayloadZip -and (Test-Path $installerPayloadZipPath)) {
    Remove-Item -LiteralPath $installerPayloadZipPath -Force
  }
}

function Invoke-InstallerWailsBuild {
  Assert-InstallerFrontendBuilt
  Write-Host "构建自定义 Wails 安装器"
  New-DirectoryIfMissing -Path $goCacheRoot
  Push-Location $installerRoot
  try {
    $previousGoCache = $env:GOCACHE
    $env:GOCACHE = $goCacheRoot
    $buildArguments = @(
      "build",
      "-m",
      "-s",
      "-skipbindings",
      "-trimpath",
      "-platform", $Platform,
      "-webview2", "embed",
      "-o", $installerOutputName,
      "-ldflags", "-s -w -X main.AppVersion=$normalizedVersion"
    )
    & wails @buildArguments
    if ($LASTEXITCODE -ne 0) {
      throw "安装器 Wails 构建失败，退出码：$LASTEXITCODE"
    }
  } finally {
    $env:GOCACHE = $previousGoCache
    Pop-Location
  }

  if (-not (Test-Path $installerSource)) {
    throw "未找到自定义安装器产物：$installerSource"
  }
  if (Test-Path $installerTarget) {
    Remove-Item -LiteralPath $installerTarget -Force
  }
  Copy-Item -LiteralPath $installerSource -Destination $installerTarget -Force
  $checksumPath = New-Sha256File -Path $installerTarget

  $manifestPath = Join-Path $distRoot "$targetName-installer.MANIFEST.txt"
  @(
    "NexTunnel desktop installer"
    "Version: $releaseVersion"
    "ApplicationVersion: $normalizedVersion"
    "WindowsResourceVersion: $windowsResourceVersion"
    "Target: $Platform"
    "Installer: custom-wails"
    "Binary: $(Split-Path -Leaf $installerTarget)"
    "Payload: $installerPayloadZipName"
    "PayloadSHA256: $((Get-FileHash -Algorithm SHA256 -Path $installerPayloadZipPath).Hash.ToLowerInvariant())"
    "Wintun: $(if ($payloadIncludesWintun) { 'bundled; architecture verified' } else { 'missing; desktop repair required' })"
    "Signing: unsigned-alpha"
    "PrunedResources: true"
    "WebView2: embedded Evergreen bootstrapper"
  ) | Set-Content -Path $manifestPath -Encoding UTF8

  Write-Host "桌面端自定义安装包已生成：$installerTarget"
  Write-Host "SHA256：$checksumPath"
}

New-DirectoryIfMissing -Path $distRoot
Save-OfficialWintunDll
try {
  Invoke-FrontendBuild -Name "桌面端" -FrontendRoot $desktopFrontendRoot
  Invoke-DesktopWailsBuild
  New-DesktopPayloadDirectory
  New-PortableArchive
  if (-not $SkipFrontend) {
    Reset-InstallerFrontendDist
  }
  Invoke-FrontendBuild -Name "安装器" -FrontendRoot $installerFrontendRoot
  Set-InstallerPayload
  Invoke-InstallerWailsBuild
} finally {
  Restore-InstallerPayload
}
