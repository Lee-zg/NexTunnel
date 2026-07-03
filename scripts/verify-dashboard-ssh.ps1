param(
  [Parameter(Mandatory = $true)]
  [string]$SshHost,

  [string]$User = "root",

  [int]$Port = 22,

  [string]$IdentityFile = "",

  [string]$RemoteDashboardHost = "127.0.0.1",

  [int]$RemoteDashboardPort = 8080,

  [int]$LocalPort = 18080,

  [string]$RemoteEnvPath = "/etc/nextunnel/server.env",

  [string]$AllowedOrigin = "",

  [string]$ReportPath = "dist/verification/dashboard-ssh-latest.json"
)

$ErrorActionPreference = "Stop"

$repositoryRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$verifyScript = Join-Path $PSScriptRoot "verify-dashboard.ps1"
$reportFullPath = if ([System.IO.Path]::IsPathRooted($ReportPath)) {
  $ReportPath
} else {
  Join-Path $repositoryRoot $ReportPath
}

if ([string]::IsNullOrWhiteSpace($User)) {
  $User = "root"
}
if ($Port -le 0) {
  $Port = 22
}
if ($RemoteDashboardPort -le 0) {
  $RemoteDashboardPort = 8080
}
if ($LocalPort -le 0) {
  $LocalPort = 18080
}

$remoteTarget = "$User@$SshHost"

function Invoke-NativeCommand {
  param(
    [string]$Name,
    [string[]]$Arguments,
    [switch]$AllowFailure
  )

  $output = & $Name @Arguments 2>&1
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

function Get-SshBaseArguments {
  $arguments = @(
    "-p", "$Port",
    "-o", "BatchMode=yes",
    "-o", "ConnectTimeout=10",
    "-o", "StrictHostKeyChecking=accept-new"
  )
  if (-not [string]::IsNullOrWhiteSpace($IdentityFile)) {
    $arguments += @("-i", $IdentityFile)
  }
  return $arguments
}

function ConvertTo-ShSingleQuoted {
  param([string]$Value)
  return "'" + $Value.Replace("'", "'\''") + "'"
}

function Read-RemoteEnvValue {
  param([string]$Name)
  $quotedName = ConvertTo-ShSingleQuoted $Name
  $quotedEnvPath = ConvertTo-ShSingleQuoted $RemoteEnvPath
  $remoteCommand = @"
while IFS= read -r line; do
  case "`$line" in
    $Name=*) printf "%s" "`${line#$Name=}"; break;;
  esac
done < $quotedEnvPath
"@
  $sshArguments = (Get-SshBaseArguments) + @($remoteTarget, $remoteCommand)
  $result = Invoke-NativeCommand -Name "ssh" -Arguments $sshArguments
  return $result.output.Trim()
}

function Start-SshTunnel {
  $forwardSpec = "127.0.0.1:$LocalPort`:$RemoteDashboardHost`:$RemoteDashboardPort"
  $arguments = (Get-SshBaseArguments) + @(
    "-o", "ExitOnForwardFailure=yes",
    "-N",
    "-L", $forwardSpec,
    $remoteTarget
  )
  $process = Start-Process -FilePath "ssh" -ArgumentList $arguments -PassThru -WindowStyle Hidden
  Start-Sleep -Seconds 2
  if ($process.HasExited) {
    throw "SSH 隧道启动失败，退出码：$($process.ExitCode)"
  }
  return $process
}

$dashboardUser = Read-RemoteEnvValue -Name "DASHBOARD_ADMIN_USER"
if ([string]::IsNullOrWhiteSpace($dashboardUser)) {
  $dashboardUser = "admin"
}

$dashboardPassword = Read-RemoteEnvValue -Name "DASHBOARD_ADMIN_PASSWORD"
if ([string]::IsNullOrWhiteSpace($dashboardPassword)) {
  throw "未能从 $RemoteEnvPath 读取 DASHBOARD_ADMIN_PASSWORD"
}

if ([string]::IsNullOrWhiteSpace($AllowedOrigin)) {
  $remoteAllowedOrigins = Read-RemoteEnvValue -Name "DASHBOARD_ALLOWED_ORIGINS"
  if (-not [string]::IsNullOrWhiteSpace($remoteAllowedOrigins)) {
    $AllowedOrigin = ($remoteAllowedOrigins -split "," | Select-Object -First 1).Trim()
  }
}

$tunnel = Start-SshTunnel
try {
  $verifyArguments = @(
    "-NoProfile",
    "-ExecutionPolicy", "Bypass",
    "-File", $verifyScript,
    "-BaseUrl", "http://127.0.0.1:$LocalPort",
    "-Username", $dashboardUser,
    "-Password", $dashboardPassword,
    "-ReportPath", $reportFullPath
  )
  if (-not [string]::IsNullOrWhiteSpace($AllowedOrigin)) {
    $verifyArguments += @("-AllowedOrigin", $AllowedOrigin)
  }
  & pwsh @verifyArguments
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }
} finally {
  if ($null -ne $tunnel -and -not $tunnel.HasExited) {
    Stop-Process -Id $tunnel.Id -Force -ErrorAction SilentlyContinue
  }
}
