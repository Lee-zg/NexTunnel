param(
  [string]$MacHost = "10.160.166.44",
  [string]$MacUser = "lizhigang",
  [int]$MacPort = 22,
  [string]$WindowsListen = "0.0.0.0:19090",
  [string]$WindowsBaseUrl = "",
  [string]$MacListen = "0.0.0.0:19091",
  [string]$MacBaseUrl = "",
  [string]$RelayAddr = "",
  [string]$RelayToken = "",
  [string]$StunServer = "",
  [string]$WintunDllPath = "",
  [string]$ReportPath = "dist/verification/tun-windows-macos-latest.json",
  [switch]$SkipRouteApply,
  [switch]$MacUseSudo,
  [switch]$MacUseHelper,
  [switch]$KeepTemporaryAccess,
  [switch]$BootstrapOnly,
  [switch]$CleanupOnly
)

$ErrorActionPreference = "Stop"

$VERIFY_SUBNET = "10.77.0.0/30"
$REMOTE_DIR = "/tmp/nextunnel-p2p-tun-verify"
$REMOTE_BINARY = "$REMOTE_DIR/p2p-tun-verify"
$REMOTE_REPORT = "$REMOTE_DIR/macos-report.json"
$REMOTE_LOG = "$REMOTE_DIR/responder.log"
$REMOTE_PID = "$REMOTE_DIR/responder.pid"
$SSH_CONNECT_TIMEOUT_SECONDS = 8
$WINDOWS_CALLBACK_PROBE_PATH = "/nextunnel-p2p-tun-probe"

$repositoryRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$desktopRoot = Join-Path $repositoryRoot "desktop"
$temporaryRoot = if (-not [string]::IsNullOrWhiteSpace($env:NEXTUNNEL_P2P_TUN_KEY_ROOT)) {
  $env:NEXTUNNEL_P2P_TUN_KEY_ROOT
} else {
  Join-Path $repositoryRoot ".tmp\p2p-tun-verify"
}
$reportFullPath = if ([System.IO.Path]::IsPathRooted($ReportPath)) {
  $ReportPath
} else {
  Join-Path $repositoryRoot $ReportPath
}
$reportDirectory = Split-Path -Parent $reportFullPath
$windowsReportPath = Join-Path $reportDirectory "tun-windows-latest.json"
$macReportPath = Join-Path $reportDirectory "tun-macos-latest.json"
$temporaryKeyPath = Join-Path $temporaryRoot "id_ed25519_nextunnel_verify"
$temporaryPublicKeyPath = "$temporaryKeyPath.pub"
$macUserHost = "$MacUser@$MacHost"
$results = New-Object System.Collections.Generic.List[object]
$windowsReport = $null
$macReport = $null
$bootstrapCommand = ""
$macKeyLoginAvailable = $false
$lastSummaryPassed = $false
$cleanupAttempted = $false

function New-Result {
  param(
    [string]$Name,
    [bool]$Passed,
    [string]$Detail = ""
  )
  [ordered]@{
    name = $Name
    passed = $Passed
    detail = $Detail
  }
}

function Add-Result {
  param(
    [string]$Name,
    [bool]$Passed,
    [string]$Detail = ""
  )
  $results.Add((New-Result -Name $Name -Passed $Passed -Detail $Detail)) | Out-Null
}

function Invoke-NativeCommand {
  param(
    [string]$Name,
    [string[]]$Arguments,
    [string]$InputText = "",
    [switch]$AllowFailure
  )

  if ([string]::IsNullOrEmpty($InputText)) {
    $output = & $Name @Arguments 2>&1
  } else {
    $output = $InputText | & $Name @Arguments 2>&1
  }
  $exitCode = $LASTEXITCODE
  $text = ($output | ForEach-Object { $_.ToString() }) -join "`n"
  if ($exitCode -ne 0 -and -not $AllowFailure) {
    throw "$Name failed with exit code $exitCode. $text"
  }
  [ordered]@{
    exit_code = $exitCode
    output = $text
  }
}

function Test-IsAdministrator {
  if (-not ($IsWindows -or $env:OS -eq "Windows_NT")) {
    return $true
  }
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = [Security.Principal.WindowsPrincipal]::new($identity)
  return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function ConvertTo-ShSingleQuoted {
  param([string]$Value)
  return "'" + $Value.Replace("'", "'\''") + "'"
}

function Get-SshArguments {
  param([string]$RemoteCommand)
  $arguments = @(
    "-i", $temporaryKeyPath,
    "-o", "IdentitiesOnly=yes",
    "-o", "BatchMode=yes",
    "-o", "ConnectTimeout=$SSH_CONNECT_TIMEOUT_SECONDS",
    "-o", "StrictHostKeyChecking=accept-new",
    "-p", "$MacPort",
    $macUserHost
  )
  if (-not [string]::IsNullOrWhiteSpace($RemoteCommand)) {
    $arguments += $RemoteCommand
  }
  return $arguments
}

function Get-ScpArguments {
  param(
    [string]$Source,
    [string]$Target
  )
  @(
    "-i", $temporaryKeyPath,
    "-o", "IdentitiesOnly=yes",
    "-o", "BatchMode=yes",
    "-o", "ConnectTimeout=$SSH_CONNECT_TIMEOUT_SECONDS",
    "-o", "StrictHostKeyChecking=accept-new",
    "-P", "$MacPort",
    $Source,
    $Target
  )
}

function Get-LocalIPv4ForPeer {
  param([string]$PeerHost)
  $udpClient = [System.Net.Sockets.UdpClient]::new()
  try {
    $udpClient.Connect($PeerHost, 22)
    return $udpClient.Client.LocalEndPoint.Address.ToString()
  } finally {
    $udpClient.Close()
  }
}

function Get-ListenPort {
  param([string]$Listen)
  $listenValue = $Listen.Trim()
  if ($listenValue -match '^\[.*\]:(\d+)$') {
    return [int]$Matches[1]
  }
  $lastColon = $listenValue.LastIndexOf(":")
  if ($lastColon -lt 0) {
    throw "listen address does not contain a port: $Listen"
  }
  $portText = $listenValue.Substring($lastColon + 1)
  $port = 0
  if (-not [int]::TryParse($portText, [ref]$port)) {
    throw "listen address has invalid port: $Listen"
  }
  return $port
}

function Add-IPv4Candidate {
  param(
    [System.Collections.Generic.List[string]]$Candidates,
    [string]$Address
  )
  if ([string]::IsNullOrWhiteSpace($Address)) {
    return
  }
  $parsedAddress = [System.Net.IPAddress]::None
  if (-not [System.Net.IPAddress]::TryParse($Address.Trim(), [ref]$parsedAddress)) {
    return
  }
  if ($parsedAddress.AddressFamily -ne [System.Net.Sockets.AddressFamily]::InterNetwork) {
    return
  }
  $normalizedAddress = $parsedAddress.ToString()
  if (
    $normalizedAddress.StartsWith("127.") -or
    $normalizedAddress.StartsWith("169.254.") -or
    $normalizedAddress -eq "0.0.0.0" -or
    $normalizedAddress -eq "255.255.255.255"
  ) {
    return
  }
  if (-not $Candidates.Contains($normalizedAddress)) {
    $Candidates.Add($normalizedAddress) | Out-Null
  }
}

function Get-LocalIPv4CandidatesForPeer {
  param([string]$PeerHost)
  $candidates = [System.Collections.Generic.List[string]]::new()

  try {
    Add-IPv4Candidate -Candidates $candidates -Address (Get-LocalIPv4ForPeer -PeerHost $PeerHost)
  } catch {
    Add-Result -Name "windows_route_ip_detect_skipped" -Passed $true -Detail $_.Exception.Message
  }

  try {
    Get-NetIPAddress -AddressFamily IPv4 -ErrorAction Stop |
      Where-Object { $_.IPAddress -and $_.AddressState -ne "Deprecated" } |
      ForEach-Object { Add-IPv4Candidate -Candidates $candidates -Address $_.IPAddress }
  } catch {
    [System.Net.Dns]::GetHostAddresses([System.Net.Dns]::GetHostName()) |
      ForEach-Object { Add-IPv4Candidate -Candidates $candidates -Address $_.ToString() }
  }

  return @($candidates)
}

function Start-WindowsCallbackProbe {
  param([int]$Port)
  $job = Start-Job -ScriptBlock {
    param([int]$ProbePort)
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Any, $ProbePort)
    $listener.Start()
    try {
      $deadline = (Get-Date).AddSeconds(30)
      while ((Get-Date) -lt $deadline) {
        if (-not $listener.Pending()) {
          Start-Sleep -Milliseconds 100
          continue
        }
        $client = $listener.AcceptTcpClient()
        try {
          $stream = $client.GetStream()
          $stream.ReadTimeout = 1000
          $buffer = New-Object byte[] 1024
          try {
            if ($stream.DataAvailable) {
              [void]$stream.Read($buffer, 0, $buffer.Length)
            }
          } catch {
          }
          $body = "ok"
          $response = "HTTP/1.1 200 OK`r`nContent-Type: text/plain`r`nContent-Length: $($body.Length)`r`nConnection: close`r`n`r`n$body"
          $bytes = [System.Text.Encoding]::ASCII.GetBytes($response)
          $stream.Write($bytes, 0, $bytes.Length)
        } finally {
          $client.Close()
        }
      }
    } finally {
      $listener.Stop()
    }
  } -ArgumentList $Port
  Start-Sleep -Milliseconds 700
  if ($job.State -eq "Failed") {
    $probeOutput = Receive-Job -Job $job -ErrorAction SilentlyContinue | Out-String
    Remove-Job -Job $job -Force -ErrorAction SilentlyContinue
    throw "failed to start Windows callback probe on port $Port. $probeOutput"
  }
  return $job
}

function Stop-WindowsCallbackProbe {
  param([object]$Job)
  if ($null -eq $Job) {
    return
  }
  Stop-Job -Job $Job -ErrorAction SilentlyContinue
  Remove-Job -Job $Job -Force -ErrorAction SilentlyContinue
}

function Resolve-WindowsBaseUrlForMac {
  param([string]$PeerHost)
  $port = Get-ListenPort -Listen $WindowsListen
  $candidates = Get-LocalIPv4CandidatesForPeer -PeerHost $PeerHost
  if ($candidates.Count -eq 0) {
    throw "no usable local IPv4 address found for Mac callback"
  }

  $probeJob = $null
  try {
    $probeJob = Start-WindowsCallbackProbe -Port $port
    foreach ($candidate in $candidates) {
      $probeUrl = "http://$($candidate):$port$WINDOWS_CALLBACK_PROBE_PATH"
      $remoteCommand = "if command -v nc >/dev/null 2>&1; then nc -z -G 2 " + (ConvertTo-ShSingleQuoted $candidate) + " $port; else curl --noproxy '*' -fsS --connect-timeout 2 --max-time 3 " + (ConvertTo-ShSingleQuoted $probeUrl) + " >/dev/null; fi"
      $probe = Invoke-NativeCommand -Name "ssh" -Arguments (Get-SshArguments $remoteCommand) -AllowFailure
      if ($probe.exit_code -eq 0) {
        Add-Result -Name "windows_base_url_probe" -Passed $true -Detail $probeUrl
        return "http://$($candidate):$port"
      }
      Add-Result -Name "windows_base_url_probe_attempt" -Passed $true -Detail "$probeUrl exit_code=$($probe.exit_code) output=$($probe.output)"
    }
  } finally {
    Stop-WindowsCallbackProbe -Job $probeJob
  }

  throw "Mac cannot reach Windows callback probe on port $port; candidates=$($candidates -join ',')"
}

function Ensure-TemporaryKey {
  if (-not (Test-Path $temporaryRoot)) {
    New-Item -ItemType Directory -Path $temporaryRoot -Force | Out-Null
  }
  if ((Test-Path $temporaryKeyPath) -and (Test-Path $temporaryPublicKeyPath) -and (Test-TemporaryKeyReadable)) {
    Protect-TemporaryKey
    return
  }
  if (Test-Path $temporaryKeyPath) {
    Remove-Item -LiteralPath $temporaryKeyPath -Force
  }
  if (Test-Path $temporaryPublicKeyPath) {
    Remove-Item -LiteralPath $temporaryPublicKeyPath -Force
  }
  # 重新生成一次性密钥，避免继承到不兼容的工作区 ACL。
  $comment = "nextunnel-p2p-tun-verify-$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
  Invoke-NativeCommand -Name "ssh-keygen" -Arguments @("-q", "-t", "ed25519", "-N", "", "-C", $comment, "-f", $temporaryKeyPath) | Out-Null
  Protect-TemporaryKey
}

function Test-TemporaryKeyReadable {
  $currentUser = [Security.Principal.WindowsIdentity]::GetCurrent().Name
  $keyOwner = (Get-Acl -Path $temporaryKeyPath).Owner
  if ($keyOwner -ne $currentUser) {
    return $false
  }
  $readResult = Invoke-NativeCommand -Name "ssh-keygen" -Arguments @("-y", "-f", $temporaryKeyPath) -AllowFailure
  if ($readResult.exit_code -ne 0) {
    return $false
  }
  $publicKeyText = (Get-Content -Path $temporaryPublicKeyPath -Raw).Trim()
  return $readResult.output.Trim() -eq $publicKeyText
}

function Protect-TemporaryKey {
  $currentUser = [Security.Principal.WindowsIdentity]::GetCurrent().Name
  Invoke-NativeCommand -Name "icacls" -Arguments @($temporaryKeyPath, "/setowner", $currentUser) -AllowFailure | Out-Null
  $privateKeyAcl = @(
    $temporaryKeyPath,
    "/inheritance:r",
    "/grant:r", "${currentUser}:F",
    "/grant:r", "SYSTEM:F",
    "/grant:r", "BUILTIN\Administrators:F"
  )
  Invoke-NativeCommand -Name "icacls" -Arguments $privateKeyAcl | Out-Null

  if (Test-Path $temporaryPublicKeyPath) {
    $publicKeyAcl = @(
      $temporaryPublicKeyPath,
      "/inheritance:r",
      "/grant:r", "${currentUser}:R"
    )
    Invoke-NativeCommand -Name "icacls" -Arguments $publicKeyAcl | Out-Null
  }
}

function Get-BootstrapCommand {
  $escapedPublicKeyPath = $temporaryPublicKeyPath.Replace("'", "''")
  $remoteCommand = 'umask 077; mkdir -p ~/.ssh; touch ~/.ssh/authorized_keys; key=$(cat | perl -pe "s/\r//g"); grep -qxF "$key" ~/.ssh/authorized_keys || printf "%s\n" "$key" >> ~/.ssh/authorized_keys'
  "Get-Content '$escapedPublicKeyPath' | ssh -p $MacPort -o StrictHostKeyChecking=accept-new $macUserHost '$remoteCommand'"
}

function Test-MacKeyLogin {
  $result = Invoke-NativeCommand -Name "ssh" -Arguments (Get-SshArguments "uname -a") -AllowFailure
  return $result.exit_code -eq 0
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

function Get-WindowsArchitectureMachine {
  $architecture = $env:PROCESSOR_ARCHITECTURE
  if ($architecture -eq "AMD64") {
    return 0x8664
  }
  if ($architecture -eq "ARM64") {
    return 0xAA64
  }
  if ($architecture -eq "x86") {
    return 0x014C
  }
  return 0
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

function Assert-WintunDllArchitecture {
  param([string]$Path)
  $expectedMachine = Get-WindowsArchitectureMachine
  if ($expectedMachine -eq 0) {
    Add-Result -Name "wintun_dll_arch_check_skipped" -Passed $true -Detail "unknown host architecture=$env:PROCESSOR_ARCHITECTURE"
    return
  }
  $actualMachine = Get-PEDllMachine -Path $Path
  if ($actualMachine -ne $expectedMachine) {
    throw ("wintun.dll architecture mismatch: path={0} expected=0x{1:X4} actual=0x{2:X4}" -f $Path, $expectedMachine, $actualMachine)
  }
}

function Copy-WintunDllIfAvailable {
  $sourcePath = Get-WintunDllCandidatePath
  if ([string]::IsNullOrWhiteSpace($sourcePath)) {
    Add-Result -Name "wintun_dll_lookup" -Passed $true -Detail "未找到显式 wintun.dll；运行时将依赖应用目录或 System32"
    return
  }
  Assert-WintunDllArchitecture -Path $sourcePath
  $targetPath = Join-Path $temporaryRoot "wintun.dll"
  Copy-Item -LiteralPath $sourcePath -Destination $targetPath -Force
  Add-Result -Name "wintun_dll_copy" -Passed $true -Detail "$sourcePath -> $targetPath"
}

function Test-MacPasswordlessSudo {
  if (-not $MacUseSudo) {
    if ($MacUseHelper) {
      Add-Result -Name "mac_helper_requested" -Passed $true -Detail "未启用 sudo；macOS 真实 utun/路由将通过 LaunchDaemon helper 验证"
    } else {
      Add-Result -Name "mac_sudo_not_requested" -Passed $true -Detail "未启用 -MacUseSudo 或 -MacUseHelper；macOS 真实 utun/路由验证可能因权限失败，仅 P2P 候选交换可继续验证"
    }
    return
  }
  $sudoProbe = Invoke-NativeCommand -Name "ssh" -Arguments (Get-SshArguments "sudo -n true") -AllowFailure
  if ($sudoProbe.exit_code -ne 0) {
    throw "MacUseSudo requires passwordless sudo for $macUserHost. 生产建议配置授权 helper 或 LaunchDaemon；验证环境可配置 sudo -n 免密。$($sudoProbe.output)"
  }
  Add-Result -Name "mac_sudo_available" -Passed $true -Detail "sudo -n true"
}

function Build-VerifyBinaries {
  param([string]$DarwinArch)
  $windowsBinaryPath = Join-Path $temporaryRoot "p2p-tun-verify-windows.exe"
  $darwinBinaryPath = Join-Path $temporaryRoot "p2p-tun-verify-darwin-$DarwinArch"
  $previousGoCache = $env:GOCACHE
  $previousGoOS = $env:GOOS
  $previousGoArch = $env:GOARCH
  $previousCGO = $env:CGO_ENABLED

  Push-Location $desktopRoot
  try {
    if ([string]::IsNullOrWhiteSpace($env:GOCACHE)) {
      $env:GOCACHE = Join-Path $repositoryRoot ".gocache-test-desktop"
    }
    if (-not (Test-Path $env:GOCACHE)) {
      New-Item -ItemType Directory -Path $env:GOCACHE -Force | Out-Null
    }

    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    Invoke-NativeCommand -Name "go" -Arguments @("build", "-o", $windowsBinaryPath, "./cmd/p2p-tun-verify") | Out-Null
    Copy-WintunDllIfAvailable

    $env:GOOS = "darwin"
    $env:GOARCH = $DarwinArch
    $env:CGO_ENABLED = "0"
    Invoke-NativeCommand -Name "go" -Arguments @("build", "-o", $darwinBinaryPath, "./cmd/p2p-tun-verify") | Out-Null
  } finally {
    $env:GOCACHE = $previousGoCache
    if ($null -eq $previousGoOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGoOS }
    if ($null -eq $previousGoArch) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGoArch }
    if ($null -eq $previousCGO) { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue } else { $env:CGO_ENABLED = $previousCGO }
    Pop-Location
  }

  [ordered]@{
    windows = $windowsBinaryPath
    darwin = $darwinBinaryPath
  }
}

function Get-MacGoArch {
  $archResult = Invoke-NativeCommand -Name "ssh" -Arguments (Get-SshArguments "uname -m")
  $machine = $archResult.output.Trim()
  switch ($machine) {
    "arm64" { return "arm64" }
    "x86_64" { return "amd64" }
    default { throw "unsupported macOS architecture: $machine" }
  }
}

function Start-MacResponder {
  param(
    [string]$DarwinBinaryPath,
    [string]$CoordinatorUrl
  )
  Invoke-NativeCommand -Name "ssh" -Arguments (Get-SshArguments ("mkdir -p " + (ConvertTo-ShSingleQuoted $REMOTE_DIR))) | Out-Null

  $remoteTarget = ("{0}:{1}" -f $macUserHost, $REMOTE_BINARY)
  Invoke-NativeCommand -Name "scp" -Arguments (Get-ScpArguments -Source $DarwinBinaryPath -Target $remoteTarget) | Out-Null
  Invoke-NativeCommand -Name "ssh" -Arguments (Get-SshArguments ("chmod +x " + (ConvertTo-ShSingleQuoted $REMOTE_BINARY))) | Out-Null

  $remoteArgs = @(
    "responder",
    "-listen", (ConvertTo-ShSingleQuoted $MacListen),
    "-coordinator", (ConvertTo-ShSingleQuoted $CoordinatorUrl),
    "-report", (ConvertTo-ShSingleQuoted $REMOTE_REPORT)
  )
  if (-not [string]::IsNullOrWhiteSpace($StunServer)) {
    $remoteArgs += @("-stun", (ConvertTo-ShSingleQuoted $StunServer))
  }
  if ($SkipRouteApply) {
    $remoteArgs += "-skip-route-apply"
  }
  $remoteExecutable = if ($MacUseSudo) { "sudo -n ./p2p-tun-verify" } else { "./p2p-tun-verify" }
  $remoteCommand = "cd " + (ConvertTo-ShSingleQuoted $REMOTE_DIR) + " && nohup $remoteExecutable " + ($remoteArgs -join " ") + " > " + (ConvertTo-ShSingleQuoted $REMOTE_LOG) + " 2>&1 & echo `$! > " + (ConvertTo-ShSingleQuoted $REMOTE_PID)
  Invoke-NativeCommand -Name "ssh" -Arguments (Get-SshArguments $remoteCommand) | Out-Null
}

function Invoke-WindowsOrchestrator {
  param([string]$WindowsBinaryPath)
  $arguments = @(
    "orchestrator",
    "-listen", $WindowsListen,
    "-peer-url", $MacBaseUrl,
    "-report", $windowsReportPath
  )
  if (-not [string]::IsNullOrWhiteSpace($StunServer)) {
    $arguments += @("-stun", $StunServer)
  }
  if (-not [string]::IsNullOrWhiteSpace($RelayAddr)) {
    $arguments += @("-relay", $RelayAddr)
  }
  if (-not [string]::IsNullOrWhiteSpace($RelayToken)) {
    $arguments += @("-relay-token", $RelayToken)
  }
  if ($SkipRouteApply) {
    $arguments += "-skip-route-apply"
  }
  Invoke-NativeCommand -Name $WindowsBinaryPath -Arguments $arguments -AllowFailure
}

function Read-JsonIfExists {
  param([string]$Path)
  if (-not (Test-Path $Path)) {
    return $null
  }
  try {
    return Get-Content -Path $Path -Raw | ConvertFrom-Json
  } catch {
    Add-Result -Name "read_json_$([System.IO.Path]::GetFileNameWithoutExtension($Path))" -Passed $false -Detail $_.Exception.Message
    return $null
  }
}

function Fetch-MacReport {
  $remoteSource = ("{0}:{1}" -f $macUserHost, $REMOTE_REPORT)
  $fetch = Invoke-NativeCommand -Name "scp" -Arguments (Get-ScpArguments -Source $remoteSource -Target $macReportPath) -AllowFailure
  Add-Result -Name "mac_report_fetch" -Passed ($fetch.exit_code -eq 0) -Detail ("exit_code=" + $fetch.exit_code)
}

function Add-KeepTemporaryAccessResult {
  if ($KeepTemporaryAccess -and -not $CleanupOnly) {
    Add-Result -Name "mac_temporary_access_retained" -Passed $true -Detail "调试模式保留临时公钥和 $REMOTE_DIR；完成后运行 -CleanupOnly 回收"
  }
}

function Wait-ForRemoteReport {
  $deadline = (Get-Date).AddSeconds(20)
  while ((Get-Date) -lt $deadline) {
    $probe = Invoke-NativeCommand -Name "ssh" -Arguments (Get-SshArguments ("test -f " + (ConvertTo-ShSingleQuoted $REMOTE_REPORT))) -AllowFailure
    if ($probe.exit_code -eq 0) {
      Add-Result -Name "mac_report_ready" -Passed $true -Detail $REMOTE_REPORT
      return $true
    }
    Start-Sleep -Milliseconds 500
  }
  Add-Result -Name "mac_report_ready" -Passed $false -Detail "timeout waiting for $REMOTE_REPORT"
  return $false
}

function Invoke-RemoteCleanup {
  $script:cleanupAttempted = $true
  if (-not (Test-Path $temporaryPublicKeyPath)) {
    Add-Result -Name "cleanup_skipped" -Passed $false -Detail "temporary public key is missing"
    return
  }
  $temporaryPublicKeyText = (Get-Content -Path $temporaryPublicKeyPath -Raw).Trim()
  $sudoPrefix = if ($MacUseSudo) { "sudo -n " } else { "" }
  $cleanupScript = @"
set -eu
if [ -f '$REMOTE_PID' ]; then
  ${sudoPrefix}kill "`$(cat '$REMOTE_PID')" 2>/dev/null || true
fi
if [ -f "`$HOME/.ssh/authorized_keys" ]; then
  tmpfile="`$(mktemp)"
  cat | perl -pe "s/\r//g" > "`$tmpfile"
  cleanfile="`$(mktemp)"
  perl -pe "s/\r//g" < "`$HOME/.ssh/authorized_keys" > "`$cleanfile"
  grep -vxF -f "`$tmpfile" "`$cleanfile" > "`$HOME/.ssh/authorized_keys.nextunnel" || true
  rm -f "`$cleanfile"
  mv "`$HOME/.ssh/authorized_keys.nextunnel" "`$HOME/.ssh/authorized_keys"
  chmod 600 "`$HOME/.ssh/authorized_keys"
  rm -f "`$tmpfile"
fi
${sudoPrefix}rm -rf '$REMOTE_DIR'
"@
  $cleanup = Invoke-NativeCommand -Name "ssh" -Arguments (Get-SshArguments $cleanupScript) -InputText $temporaryPublicKeyText -AllowFailure
  Add-Result -Name "mac_temporary_cleanup" -Passed ($cleanup.exit_code -eq 0) -Detail ("exit_code=" + $cleanup.exit_code)
}

function Write-Summary {
  if (-not (Test-Path $reportDirectory)) {
    New-Item -ItemType Directory -Path $reportDirectory -Force | Out-Null
  }
  $failedCount = ($results | Where-Object { -not $_.passed }).Count
  $summary = [ordered]@{
    generated_at = [DateTimeOffset]::UtcNow.ToString("o")
    mode = "windows_macos_p2p_tun"
    virtual_subnet = $VERIFY_SUBNET
    mac = [ordered]@{
      host = $MacHost
      user = $MacUser
      port = $MacPort
      base_url = $MacBaseUrl
      use_helper = [bool]$MacUseHelper
      use_sudo = [bool]$MacUseSudo
    }
    windows = [ordered]@{
      listen = $WindowsListen
      base_url = $WindowsBaseUrl
    }
    relay_configured = (-not [string]::IsNullOrWhiteSpace($RelayAddr))
    passed = ($failedCount -eq 0)
    results = $results
    reports = [ordered]@{
      windows = $windowsReportPath
      macos = $macReportPath
    }
    windows_report = $windowsReport
    macos_report = $macReport
  }
  if (-not [string]::IsNullOrWhiteSpace($bootstrapCommand)) {
    $summary["bootstrap_command"] = $bootstrapCommand
  }
  $script:lastSummaryPassed = [bool]$summary.passed
  $json = $summary | ConvertTo-Json -Depth 12
  $json | Set-Content -Path $reportFullPath -Encoding UTF8
  Write-Output $json
}

try {
  if (-not (Test-Path $reportDirectory)) {
    New-Item -ItemType Directory -Path $reportDirectory -Force | Out-Null
  }
  Ensure-TemporaryKey
  $bootstrapCommand = Get-BootstrapCommand

  if ($BootstrapOnly) {
    Add-Result -Name "bootstrap_command_generated" -Passed $true -Detail $bootstrapCommand
    return
  }

  if (-not (Test-IsAdministrator) -and -not $CleanupOnly) {
    Add-Result -Name "windows_admin" -Passed $false -Detail "请用管理员权限运行 PowerShell 后重试"
    return
  }
  Add-Result -Name "windows_admin" -Passed $true -Detail "administrator=true"

  if (-not (Test-MacKeyLogin)) {
    Add-Result -Name "mac_ssh_key_login" -Passed $false -Detail "临时公钥尚未写入 Mac；先执行 bootstrap_command 后重试"
    return
  }
  $macKeyLoginAvailable = $true
  Add-Result -Name "mac_ssh_key_login" -Passed $true -Detail $macUserHost

  if ($CleanupOnly) {
    Invoke-RemoteCleanup
    return
  }
  Test-MacPasswordlessSudo

  if ([string]::IsNullOrWhiteSpace($WindowsBaseUrl)) {
    $WindowsBaseUrl = Resolve-WindowsBaseUrlForMac -PeerHost $MacHost
  } else {
    Add-Result -Name "windows_base_url_configured" -Passed $true -Detail $WindowsBaseUrl
  }
  if ([string]::IsNullOrWhiteSpace($MacBaseUrl)) {
    $MacBaseUrl = "http://$($MacHost):19091"
  }

  $macGoArch = Get-MacGoArch
  Add-Result -Name "mac_arch_detect" -Passed $true -Detail $macGoArch

  $binaries = Build-VerifyBinaries -DarwinArch $macGoArch
  Add-Result -Name "build_verify_binaries" -Passed $true -Detail "windows=$($binaries.windows); darwin=$($binaries.darwin)"

  Start-MacResponder -DarwinBinaryPath $binaries.darwin -CoordinatorUrl $WindowsBaseUrl
  Add-Result -Name "mac_responder_start" -Passed $true -Detail "$MacBaseUrl -> $WindowsBaseUrl"
  Start-Sleep -Seconds 2

  $orchestrator = Invoke-WindowsOrchestrator -WindowsBinaryPath $binaries.windows
  Add-Result -Name "windows_orchestrator" -Passed ($orchestrator.exit_code -eq 0) -Detail ("exit_code=" + $orchestrator.exit_code)

  if (Wait-ForRemoteReport) {
    Fetch-MacReport
  }
  $windowsReport = Read-JsonIfExists -Path $windowsReportPath
  $macReport = Read-JsonIfExists -Path $macReportPath
  if ($null -ne $windowsReport) {
    Add-Result -Name "windows_report_passed" -Passed ([bool]$windowsReport.passed) -Detail $windowsReportPath
  }
  if ($null -ne $macReport) {
    Add-Result -Name "macos_report_passed" -Passed ([bool]$macReport.passed) -Detail $macReportPath
  }
} catch {
  Add-Result -Name "verify_p2p_tun_unhandled_error" -Passed $false -Detail $_.Exception.Message
} finally {
  if (-not $BootstrapOnly -and -not $KeepTemporaryAccess -and $macKeyLoginAvailable -and -not $cleanupAttempted) {
    Invoke-RemoteCleanup
  }
  Add-KeepTemporaryAccessResult
  Write-Summary
  if (-not $script:lastSummaryPassed) {
    exit 1
  }
}
