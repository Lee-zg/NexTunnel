param(
  [string]$ReportPath = "dist/verification/verification-scripts-static-latest.json"
)

$ErrorActionPreference = "Stop"

$repositoryRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$reportFullPath = if ([System.IO.Path]::IsPathRooted($ReportPath)) {
  $ReportPath
} else {
  Join-Path $repositoryRoot $ReportPath
}
$reportDirectory = Split-Path -Parent $reportFullPath
if (-not (Test-Path $reportDirectory)) {
  New-Item -ItemType Directory -Path $reportDirectory -Force | Out-Null
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

function Convert-DefaultExpressionText {
  param([object]$DefaultValue)

  if ($null -eq $DefaultValue) {
    return ""
  }
  $text = $DefaultValue.Extent.Text.Trim()
  if ($text.Length -ge 2) {
    $first = $text.Substring(0, 1)
    $last = $text.Substring($text.Length - 1, 1)
    if (($first -eq '"' -and $last -eq '"') -or ($first -eq "'" -and $last -eq "'")) {
      return $text.Substring(1, $text.Length - 2)
    }
  }
  return $text
}

function Test-ParameterMandatory {
  param([object]$Parameter)

  foreach ($attribute in $Parameter.Attributes) {
    if ($attribute -isnot [System.Management.Automation.Language.AttributeAst]) {
      continue
    }
    if ($attribute.TypeName.FullName -notin @("Parameter", "ParameterAttribute", "System.Management.Automation.ParameterAttribute")) {
      continue
    }
    foreach ($namedArgument in $attribute.NamedArguments) {
      if ($namedArgument.ArgumentName -ne "Mandatory") {
        continue
      }
      if ($null -eq $namedArgument.Argument) {
        return $true
      }
      $argumentText = $namedArgument.Argument.Extent.Text.Trim()
      return ($argumentText -eq '$true' -or $argumentText -eq 'true')
    }
  }
  return $false
}

function Get-ScriptParameterMap {
  param([string]$Path)

  $tokens = $null
  $parseErrors = $null
  $ast = [System.Management.Automation.Language.Parser]::ParseFile($Path, [ref]$tokens, [ref]$parseErrors)
  if ($parseErrors.Count -ne 0) {
    throw "cannot inspect parameters because the script has parse errors"
  }

  $parameters = @{}
  if ($null -eq $ast.ParamBlock) {
    return $parameters
  }

  foreach ($parameter in $ast.ParamBlock.Parameters) {
    $name = $parameter.Name.VariablePath.UserPath
    $parameters[$name] = [ordered]@{
      type = if ($null -ne $parameter.StaticType) { $parameter.StaticType.Name } else { "" }
      default = Convert-DefaultExpressionText -DefaultValue $parameter.DefaultValue
      mandatory = Test-ParameterMandatory -Parameter $parameter
    }
  }
  return $parameters
}

function Test-ParameterContract {
  param(
    [string]$RelativePath,
    [object[]]$Requirements
  )

  $scriptPath = Join-Path $repositoryRoot $RelativePath
  try {
    $parameters = Get-ScriptParameterMap -Path $scriptPath
    $failures = New-Object System.Collections.Generic.List[string]
    foreach ($requirement in $Requirements) {
      $name = $requirement.Name
      if (-not $parameters.ContainsKey($name)) {
        $failures.Add("missing parameter $name") | Out-Null
        continue
      }
      $actual = $parameters[$name]
      if ($requirement.ContainsKey("Type") -and $actual.type -ne $requirement.Type) {
        $failures.Add("$name type expected=$($requirement.Type) actual=$($actual.type)") | Out-Null
      }
      if ($requirement.ContainsKey("Default") -and $actual.default -ne $requirement.Default) {
        $failures.Add("$name default expected=$($requirement.Default) actual=$($actual.default)") | Out-Null
      }
      if ($requirement.ContainsKey("Mandatory") -and $actual.mandatory -ne [bool]$requirement.Mandatory) {
        $failures.Add("$name mandatory expected=$($requirement.Mandatory) actual=$($actual.mandatory)") | Out-Null
      }
    }

    if ($failures.Count -eq 0) {
      $results.Add((New-Result -Name "parameter_contract:$RelativePath" -Passed $true -Detail "ok")) | Out-Null
    } else {
      $results.Add((New-Result -Name "parameter_contract:$RelativePath" -Passed $false -Detail ($failures -join "; "))) | Out-Null
    }
  } catch {
    $results.Add((New-Result -Name "parameter_contract:$RelativePath" -Passed $false -Detail $_.Exception.Message)) | Out-Null
  }
}

$results = New-Object System.Collections.Generic.List[object]

$powerShellScripts = Get-ChildItem -Path (Join-Path $repositoryRoot "scripts") -Filter "*.ps1" -File | Sort-Object FullName
foreach ($script in $powerShellScripts) {
  $tokens = $null
  $parseErrors = $null
  [System.Management.Automation.Language.Parser]::ParseFile($script.FullName, [ref]$tokens, [ref]$parseErrors) | Out-Null
  $relativePath = [System.IO.Path]::GetRelativePath($repositoryRoot, $script.FullName)
  if ($parseErrors.Count -eq 0) {
    $results.Add((New-Result -Name "powershell_parse:$relativePath" -Passed $true -Detail "ok")) | Out-Null
  } else {
    $detail = ($parseErrors | ForEach-Object { "$($_.Extent.StartLineNumber):$($_.Extent.StartColumnNumber) $($_.Message)" }) -join "; "
    $results.Add((New-Result -Name "powershell_parse:$relativePath" -Passed $false -Detail $detail)) | Out-Null
  }
}

$parameterContracts = @(
  @{
    Script = "scripts/package-cli.ps1"
    Parameters = @(
      @{ Name = "Version"; Type = "String"; Default = "v0.6.4-alpha" }
    )
  },
  @{
    Script = "scripts/package-server.ps1"
    Parameters = @(
      @{ Name = "Version"; Type = "String"; Default = "v0.6.4-alpha" },
      @{ Name = "SkipWeb"; Type = "SwitchParameter" }
    )
  },
  @{
    Script = "scripts/package-desktop.ps1"
    Parameters = @(
      @{ Name = "Version"; Type = "String"; Default = "v0.6.4-alpha" },
      @{ Name = "Platform"; Type = "String"; Default = "windows/amd64" },
      @{ Name = "WintunMode"; Type = "String"; Default = "bundled" },
      @{ Name = "WintunDllPath"; Type = "String" },
      @{ Name = "WintunSha256"; Type = "String"; Default = "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51" },
      @{ Name = "SkipFrontend"; Type = "SwitchParameter" },
      @{ Name = "SkipZip"; Type = "SwitchParameter" }
    )
  },
  @{
    Script = "scripts/verify-dashboard.ps1"
    Parameters = @(
      @{ Name = "BaseUrl"; Type = "String"; Mandatory = $true },
      @{ Name = "Password"; Type = "String"; Mandatory = $true },
      @{ Name = "ReportPath"; Type = "String"; Default = "dist/verification/dashboard-https-latest.json" },
      @{ Name = "AllowInsecureHttpCredentials"; Type = "SwitchParameter" }
    )
  },
  @{
    Script = "scripts/verify-dashboard-ssh.ps1"
    Parameters = @(
      @{ Name = "SshHost"; Type = "String"; Mandatory = $true },
      @{ Name = "ReportPath"; Type = "String"; Default = "dist/verification/dashboard-ssh-latest.json" }
    )
  },
  @{
    Script = "scripts/verify-p2p-tun.ps1"
    Parameters = @(
      @{ Name = "MacHost"; Type = "String"; Default = "10.160.166.44" },
      @{ Name = "MacUser"; Type = "String"; Default = "lizhigang" },
      @{ Name = "WintunDllPath"; Type = "String" },
      @{ Name = "ReportPath"; Type = "String"; Default = "dist/verification/tun-windows-macos-latest.json" },
      @{ Name = "MacUseSudo"; Type = "SwitchParameter" },
      @{ Name = "MacUseHelper"; Type = "SwitchParameter" },
      @{ Name = "BootstrapOnly"; Type = "SwitchParameter" },
      @{ Name = "CleanupOnly"; Type = "SwitchParameter" }
    )
  },
  @{
    Script = "scripts/verify-tun.ps1"
    Parameters = @(
      @{ Name = "ReportPath"; Type = "String"; Default = "dist/verification/tun-local-latest.json" },
      @{ Name = "SkipRouteApply"; Type = "SwitchParameter" }
    )
  },
  @{
    Script = "scripts/verify-edge-rehearsal.ps1"
    Parameters = @(
      @{ Name = "ReportPath"; Type = "String"; Default = "dist/verification/edge-rehearsal-latest.json" },
      @{ Name = "RegisterRemote"; Type = "SwitchParameter" }
    )
  }
)

foreach ($contract in $parameterContracts) {
  Test-ParameterContract -RelativePath $contract.Script -Requirements $contract.Parameters
}

$bashScripts = @(
  "scripts/package-macos.sh",
  "scripts/verify-ebpf-linux.sh"
)
$bash = Get-Command "bash" -ErrorAction SilentlyContinue
if ($null -eq $bash) {
  $results.Add((New-Result -Name "bash_syntax" -Passed $true -Detail "bash not found; skipped")) | Out-Null
} else {
  foreach ($relativePath in $bashScripts) {
    $scriptPath = Join-Path $repositoryRoot $relativePath
    if (-not (Test-Path $scriptPath)) {
      $results.Add((New-Result -Name "bash_syntax:$relativePath" -Passed $false -Detail "file not found")) | Out-Null
      continue
    }
    $output = & $bash.Source -n $scriptPath 2>&1
    $exitCode = $LASTEXITCODE
    $detail = ($output | ForEach-Object { $_.ToString() }) -join "`n"
    if ([string]::IsNullOrWhiteSpace($detail)) {
      $detail = "ok"
    }
    $results.Add((New-Result -Name "bash_syntax:$relativePath" -Passed ($exitCode -eq 0) -Detail $detail)) | Out-Null
  }
}

$summary = [ordered]@{
  generated_at = [DateTimeOffset]::UtcNow.ToString("o")
  report_path = $reportFullPath
  passed = ($results | Where-Object { -not $_.passed }).Count -eq 0
  results = $results
}

$json = $summary | ConvertTo-Json -Depth 8
$json | Set-Content -Path $reportFullPath -Encoding UTF8
Write-Output $json

if (-not $summary.passed) {
  exit 1
}
