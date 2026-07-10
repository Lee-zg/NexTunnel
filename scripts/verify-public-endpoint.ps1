param(
  [Parameter(Mandatory = $true)]
  [string]$GatewayUrl,

  [string]$HostHeader = "",

  [string]$ExpectedContains = "",

  [string]$BasicUsername = "",

  [string]$BasicPassword = "",

  [string]$BearerToken = "",

  [string]$DashboardUrl = "",

  [string]$DashboardToken = "",

  [int]$RequestLogLimit = 20,

  [string]$ReportPath = "dist/verification/public-endpoint-latest.json",

  [switch]$AllowInsecureHttpCredentials
)

$ErrorActionPreference = "Stop"

$CHECK_TIMEOUT_SECONDS = 15
$repositoryRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
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
  return $normalizedHost.StartsWith("127.")
}

function Assert-CredentialTransportSafe {
  param([string]$Url)
  $parsed = [System.Uri]$Url
  if ($parsed.Scheme -eq "https") {
    return
  }
  if ($parsed.Scheme -eq "http" -and (Test-IsLoopbackHost -HostName $parsed.Host)) {
    return
  }
  if ($AllowInsecureHttpCredentials) {
    return
  }
  throw "拒绝通过非本机 HTTP 发送 Public Endpoint 认证凭据。请改用 HTTPS，或仅在隔离测试环境显式传入 -AllowInsecureHttpCredentials。"
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

function New-EndpointHeaders {
  param(
    [string]$AuthMode = ""
  )
  $headers = @{}
  if (-not [string]::IsNullOrWhiteSpace($HostHeader)) {
    $headers["Host"] = $HostHeader
  }
  if ($AuthMode -eq "basic") {
    $plain = "$BasicUsername`:$BasicPassword"
    $encoded = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes($plain))
    $headers["Authorization"] = "Basic $encoded"
  }
  if ($AuthMode -eq "bearer") {
    $headers["Authorization"] = "Bearer $BearerToken"
  }
  return $headers
}

function Invoke-Endpoint {
  param(
    [string]$AuthMode = ""
  )
  $request = @{
    Method = "GET"
    Uri = $GatewayUrl
    Headers = (New-EndpointHeaders -AuthMode $AuthMode)
    TimeoutSec = $CHECK_TIMEOUT_SECONDS
  }
  Invoke-WebRequest @request
}

function Invoke-DashboardJson {
  param([string]$Path)
  if ([string]::IsNullOrWhiteSpace($DashboardUrl) -or [string]::IsNullOrWhiteSpace($DashboardToken)) {
    return $null
  }
  $request = @{
    Method = "GET"
    Uri = ($DashboardUrl.TrimEnd("/") + $Path)
    Headers = @{ Authorization = "Bearer $DashboardToken" }
    TimeoutSec = $CHECK_TIMEOUT_SECONDS
  }
  $response = Invoke-WebRequest @request
  if ([string]::IsNullOrWhiteSpace($response.Content)) {
    return $null
  }
  return $response.Content | ConvertFrom-Json
}

$results = New-Object System.Collections.Generic.List[object]

try {
  if (-not [string]::IsNullOrWhiteSpace($BasicUsername) -or -not [string]::IsNullOrWhiteSpace($BearerToken)) {
    Assert-CredentialTransportSafe -Url $GatewayUrl
  }

  if (-not [string]::IsNullOrWhiteSpace($BasicUsername)) {
    try {
      Invoke-Endpoint | Out-Null
      $results.Add((New-Result "public_endpoint_basic_auth_rejects_missing_credentials" $false "missing credentials unexpectedly succeeded"))
    } catch {
      $statusCode = $_.Exception.Response.StatusCode.value__
      $results.Add((New-Result "public_endpoint_basic_auth_rejects_missing_credentials" ($statusCode -eq 401) "HTTP $statusCode"))
    }
    $response = Invoke-Endpoint -AuthMode "basic"
  } elseif (-not [string]::IsNullOrWhiteSpace($BearerToken)) {
    try {
      Invoke-Endpoint | Out-Null
      $results.Add((New-Result "public_endpoint_bearer_rejects_missing_token" $false "missing bearer token unexpectedly succeeded"))
    } catch {
      $statusCode = $_.Exception.Response.StatusCode.value__
      $results.Add((New-Result "public_endpoint_bearer_rejects_missing_token" ($statusCode -eq 401) "HTTP $statusCode"))
    }
    $response = Invoke-Endpoint -AuthMode "bearer"
  } else {
    $response = Invoke-Endpoint
  }

  $statusPass = $response.StatusCode -ge 200 -and $response.StatusCode -lt 400
  $bodyPass = $true
  if (-not [string]::IsNullOrWhiteSpace($ExpectedContains)) {
    $bodyPass = $response.Content.Contains($ExpectedContains)
  }
  $results.Add((New-Result "public_endpoint_gateway_access" ($statusPass -and $bodyPass) "HTTP $($response.StatusCode) expected_contains=$ExpectedContains"))

  $endpoints = Invoke-DashboardJson -Path "/api/v1/endpoints"
  if ($null -ne $endpoints) {
    $items = @($endpoints.data.items)
    $matched = $false
    foreach ($item in $items) {
      if ((-not [string]::IsNullOrWhiteSpace($HostHeader) -and $item.domain -eq $HostHeader) -or
          ([string]::IsNullOrWhiteSpace($HostHeader) -and $GatewayUrl.StartsWith([string]$item.public_url))) {
        $matched = $true
      }
    }
    $results.Add((New-Result "dashboard_endpoint_visible" ($endpoints.success -eq $true -and $endpoints.data.available -eq $true -and $matched) "available=$($endpoints.data.available) endpoint_count=$($items.Count)"))
  }

  $requestLogs = Invoke-DashboardJson -Path "/api/v1/http-requests?limit=$RequestLogLimit"
  if ($null -ne $requestLogs) {
    $logs = @($requestLogs.data.items)
    $logMatched = $false
    foreach ($log in $logs) {
      if ([string]::IsNullOrWhiteSpace($HostHeader) -or $log.host -eq $HostHeader) {
        $logMatched = $true
        break
      }
    }
    $results.Add((New-Result "dashboard_request_log_visible" ($requestLogs.success -eq $true -and $requestLogs.data.available -eq $true -and $logMatched) "available=$($requestLogs.data.available) log_count=$($logs.Count)"))
  }
} catch {
  $results.Add((New-Result "public_endpoint_verification_unhandled_error" $false $_.Exception.Message))
}

$summary = [ordered]@{
  generated_at = [DateTimeOffset]::UtcNow.ToString("o")
  gateway_url = $GatewayUrl
  host_header = $HostHeader
  dashboard_url = $DashboardUrl
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
