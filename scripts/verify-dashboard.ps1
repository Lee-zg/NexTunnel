param(
  [Parameter(Mandatory = $true)]
  [string]$BaseUrl,

  [string]$Username = "admin",

  [Parameter(Mandatory = $true)]
  [string]$Password,

  [string]$AllowedOrigin = "",

  [string]$RejectedOrigin = "https://evil.example.invalid",

  [string]$ReportPath = "dist/verification/dashboard-https-latest.json",

  [switch]$AllowInsecureHttpCredentials
)

$ErrorActionPreference = "Stop"

$CHECK_TIMEOUT_SECONDS = 15
$VERIFY_ACL_ID = "verify-dashboard-" + [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$VERIFY_ALERT_RULE_ID = "verify-alert-" + [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$repositoryRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
if ([string]::IsNullOrWhiteSpace($ReportPath)) {
  $ReportPath = "dist/verification/dashboard-https-latest.json"
}
$reportFullPath = if ([System.IO.Path]::IsPathRooted($ReportPath)) {
  $ReportPath
} else {
  Join-Path $repositoryRoot $ReportPath
}

function Test-IsLoopbackHost {
  param([string]$HostName)
  $normalizedHost = $HostName.Trim().TrimEnd(".").ToLowerInvariant()
  if ($normalizedHost -eq "localhost" -or $normalizedHost -eq "::1") {
    return $true
  }
  if ($normalizedHost.StartsWith("127.")) {
    return $true
  }
  return $false
}

function Assert-CredentialTransportSafe {
  $parsedBaseUrl = [System.Uri]$BaseUrl
  if ($parsedBaseUrl.Scheme -eq "https") {
    return
  }
  if ($parsedBaseUrl.Scheme -eq "http" -and (Test-IsLoopbackHost -HostName $parsedBaseUrl.Host)) {
    return
  }
  if ($AllowInsecureHttpCredentials) {
    return
  }
  throw "拒绝通过非本机 HTTP 向 Dashboard 发送管理员密码。请改用 HTTPS，或使用 scripts/verify-dashboard-ssh.ps1 通过 SSH 隧道验证；仅在隔离测试环境可显式传入 -AllowInsecureHttpCredentials。"
}

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

function Invoke-JsonRequest {
  param(
    [string]$Method,
    [string]$Path,
    [object]$Body = $null,
    [string]$Token = "",
    [string]$Origin = ""
  )

  $headers = @{}
  if (-not [string]::IsNullOrWhiteSpace($Token)) {
    $headers["Authorization"] = "Bearer $Token"
  }
  if (-not [string]::IsNullOrWhiteSpace($Origin)) {
    $headers["Origin"] = $Origin
  }

  $request = @{
    Method = $Method
    Uri = ($BaseUrl.TrimEnd("/") + $Path)
    Headers = $headers
    TimeoutSec = $CHECK_TIMEOUT_SECONDS
  }
  if ($null -ne $Body) {
    $request["ContentType"] = "application/json"
    $request["Body"] = ($Body | ConvertTo-Json -Depth 8)
  }
  Invoke-WebRequest @request
}

function Convert-ApiData {
  param([object]$Response)
  if ([string]::IsNullOrWhiteSpace($Response.Content)) {
    return $null
  }
  $payload = $Response.Content | ConvertFrom-Json
  if ($payload.PSObject.Properties.Name -contains "data") {
    return $payload.data
  }
  return $payload
}

function Get-HeaderValue {
  param(
    [object]$Response,
    [string]$Name
  )
  $value = $Response.Headers[$Name]
  if ($null -eq $value) {
    return ""
  }
  if ($value -is [System.Array]) {
    return [string]$value[0]
  }
  return [string]$value
}

$results = New-Object System.Collections.Generic.List[object]
$token = ""

try {
  Assert-CredentialTransportSafe

  $health = Invoke-JsonRequest -Method "GET" -Path "/api/v1/health"
  $healthData = Convert-ApiData $health
  $results.Add((New-Result "dashboard_health" ($health.StatusCode -eq 200 -and $healthData.status -eq "ok") "HTTP $($health.StatusCode) status=$($healthData.status)"))

  if (-not [string]::IsNullOrWhiteSpace($AllowedOrigin)) {
    $cors = Invoke-JsonRequest -Method "OPTIONS" -Path "/api/v1/nodes" -Origin $AllowedOrigin
    $allowOrigin = Get-HeaderValue -Response $cors -Name "Access-Control-Allow-Origin"
    $results.Add((New-Result "dashboard_cors_allowed_origin" ($allowOrigin -eq $AllowedOrigin) "allow-origin=$allowOrigin"))
  }

  $rejectedCors = Invoke-JsonRequest -Method "OPTIONS" -Path "/api/v1/nodes" -Origin $RejectedOrigin
  $rejectedAllowOrigin = Get-HeaderValue -Response $rejectedCors -Name "Access-Control-Allow-Origin"
  $results.Add((New-Result "dashboard_cors_rejected_origin" ([string]::IsNullOrWhiteSpace($rejectedAllowOrigin)) "allow-origin=$rejectedAllowOrigin"))

  $login = Invoke-JsonRequest -Method "POST" -Path "/api/v1/auth/login" -Body @{ username = $Username; password = $Password }
  $loginData = Convert-ApiData $login
  $token = [string]$loginData.token
  $results.Add((New-Result "dashboard_login" (-not [string]::IsNullOrWhiteSpace($token)) "user=$($loginData.user.username) role=$($loginData.user.role)"))

  try {
    Invoke-JsonRequest -Method "GET" -Path "/api/v1/nodes" -Token "invalid-token-for-verification" | Out-Null
    $results.Add((New-Result "dashboard_invalid_token_401" $false "invalid token unexpectedly succeeded"))
  } catch {
    $statusCode = $_.Exception.Response.StatusCode.value__
    $results.Add((New-Result "dashboard_invalid_token_401" ($statusCode -eq 401) "HTTP $statusCode"))
  }

  $nodes = Invoke-JsonRequest -Method "GET" -Path "/api/v1/nodes" -Token $token
  $results.Add((New-Result "dashboard_nodes_api" ($nodes.StatusCode -eq 200) "HTTP $($nodes.StatusCode)"))

  $stats = Invoke-JsonRequest -Method "GET" -Path "/api/v1/stats" -Token $token
  $results.Add((New-Result "dashboard_stats_api" ($stats.StatusCode -eq 200) "HTTP $($stats.StatusCode)"))

  $clients = Invoke-JsonRequest -Method "GET" -Path "/api/v1/clients" -Token $token
  $clientsData = Convert-ApiData $clients
  $clientProperties = if ($null -ne $clientsData) { @($clientsData.PSObject.Properties.Name) } else { @() }
  $hasClientsArray = $clientProperties -contains "clients"
  $clientCount = if ($hasClientsArray) { @($clientsData.clients).Count } else { 0 }
  $results.Add((New-Result "dashboard_clients_api" ($clients.StatusCode -eq 200 -and $hasClientsArray) "configured=$($clientsData.configured) available=$($clientsData.available) clients=$clientCount error=$($clientsData.error)"))

  $aclBody = @{
    id = $VERIFY_ACL_ID
    source = "*"
    target = "verify-target"
    action = "allow"
    protocol = "tcp"
    priority = 9999
    enabled = $true
  }
  $createAcl = Invoke-JsonRequest -Method "POST" -Path "/api/v1/acl" -Body $aclBody -Token $token
  $results.Add((New-Result "dashboard_acl_create" ($createAcl.StatusCode -eq 201) "id=$VERIFY_ACL_ID"))

  $deleteAcl = Invoke-JsonRequest -Method "DELETE" -Path "/api/v1/acl/$VERIFY_ACL_ID" -Token $token
  $results.Add((New-Result "dashboard_acl_delete" ($deleteAcl.StatusCode -eq 200) "id=$VERIFY_ACL_ID"))

  $alertRuleBody = @{
    id = $VERIFY_ALERT_RULE_ID
    name = "verification rule"
    description = "dashboard e2e verification"
    condition = "high_latency"
    threshold = 1
    level = "warning"
    enabled = $true
    cooldown = 0
  }
  $createRule = Invoke-JsonRequest -Method "POST" -Path "/api/v1/alert-rules" -Body $alertRuleBody -Token $token
  $results.Add((New-Result "dashboard_alert_rule_create" ($createRule.StatusCode -eq 201) "id=$VERIFY_ALERT_RULE_ID"))

  $metricsBody = @{
    samples = @(@{
      NodeID = "verify-node"
      Metric = "high_latency"
      Value = 10
    })
  }
  $metrics = Invoke-JsonRequest -Method "POST" -Path "/api/v1/metrics" -Body $metricsBody -Token $token
  $metricsData = Convert-ApiData $metrics
  $results.Add((New-Result "dashboard_metrics_ingest" ($metrics.StatusCode -eq 200 -and $metricsData.ingested -eq 1) "ingested=$($metricsData.ingested) fired=$($metricsData.fired)"))

  $alerts = Invoke-JsonRequest -Method "GET" -Path "/api/v1/alerts" -Token $token
  $results.Add((New-Result "dashboard_alerts_api" ($alerts.StatusCode -eq 200) "HTTP $($alerts.StatusCode)"))

  $deleteRule = Invoke-JsonRequest -Method "DELETE" -Path "/api/v1/alert-rules/$VERIFY_ALERT_RULE_ID" -Token $token
  $results.Add((New-Result "dashboard_alert_rule_delete" ($deleteRule.StatusCode -eq 200) "id=$VERIFY_ALERT_RULE_ID"))

  try {
    $root = Invoke-JsonRequest -Method "GET" -Path "/"
    $results.Add((New-Result "dashboard_static_entrypoint" ($root.StatusCode -eq 200) "HTTP $($root.StatusCode)"))
  } catch {
    $statusCode = $_.Exception.Response.StatusCode.value__
    $results.Add((New-Result "dashboard_static_entrypoint" ($statusCode -eq 404) "HTTP $statusCode; static assets may be served by the reverse proxy"))
  }
} catch {
  $results.Add((New-Result "dashboard_verification_unhandled_error" $false $_.Exception.Message))
}

$summary = [ordered]@{
  generated_at = [DateTimeOffset]::UtcNow.ToString("o")
  base_url = $BaseUrl
  report_path = $reportFullPath
  passed = ($results | Where-Object { -not $_.passed }).Count -eq 0
  results = $results
}

$json = $summary | ConvertTo-Json -Depth 8
if (-not [string]::IsNullOrWhiteSpace($reportFullPath)) {
  $reportDirectory = Split-Path -Parent $reportFullPath
  if (-not [string]::IsNullOrWhiteSpace($reportDirectory)) {
    New-Item -ItemType Directory -Path $reportDirectory -Force | Out-Null
  }
  $json | Set-Content -Path $reportFullPath -Encoding UTF8
}

Write-Output $json
if (-not $summary.passed) {
  exit 1
}
