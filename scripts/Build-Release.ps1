#requires -Version 5.1
# Build Zentrola Backend, Admin Web, or both with an interactive target selector.
[CmdletBinding()]
param(
    [ValidateSet('backend', 'web', 'all')]
    [string]$Component,

    [ValidateSet('windows', 'macos', 'linux')]
    [string]$TargetOS,

    [ValidateSet('amd64', 'arm64', '386')]
    [string]$Architecture,

    [switch]$SkipNpmInstall
)

$ErrorActionPreference = 'Stop'

function Read-ReleaseChoice {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Title,

        [Parameter(Mandatory = $true)]
        [string[]]$Options
    )

    Write-Host ''
    Write-Host $Title -ForegroundColor Cyan
    for ($index = 0; $index -lt $Options.Count; $index++) {
        Write-Host ('  [{0}] {1}' -f ($index + 1), $Options[$index])
    }
    while ($true) {
        $selection = Read-Host ('Select 1-{0}' -f $Options.Count)
        $parsed = 0
        if ([int]::TryParse($selection, [ref]$parsed) -and $parsed -ge 1 -and $parsed -le $Options.Count) {
            return $Options[$parsed - 1]
        }
        Write-Host 'Invalid selection. Try again.' -ForegroundColor Yellow
    }
}

if (!$Component) {
    $Component = Read-ReleaseChoice 'Select release component' @('backend', 'web', 'all')
}
if (!$TargetOS) {
    $TargetOS = Read-ReleaseChoice 'Select target operating system' @('windows', 'macos', 'linux')
}
if (!$Architecture) {
    $architectures = @('amd64', 'arm64', '386')
    if ($TargetOS -eq 'macos') {
        $architectures = @('amd64', 'arm64')
    }
    $Architecture = Read-ReleaseChoice 'Select target CPU architecture' $architectures
}

$projectRoot = Split-Path -Parent $PSScriptRoot
$goCommand = Get-Command go -ErrorAction SilentlyContinue
if (!$goCommand) {
    throw 'Go was not found. Install Go and add it to PATH.'
}
$goExecutable = $goCommand.Source

$previousNpmPrefix = $env:NPM_CONFIG_PREFIX
$npmPrefixChanged = $false
if ($Component -ne 'backend') {
    $npmCommand = Get-Command npm.cmd -ErrorAction SilentlyContinue
    $nodeCommand = Get-Command node.exe -ErrorAction SilentlyContinue
    if (!$npmCommand -or !$nodeCommand) {
        throw 'Building Admin Web requires Node.js and npm in PATH.'
    }

    # Prefer npm bundled with Node when a broken user-level npm shadows it.
    $nodeRoot = Split-Path -Parent $nodeCommand.Source
    $bundledNpm = Join-Path $nodeRoot 'node_modules\npm\bin\npm-cli.js'
    if (Test-Path -LiteralPath $bundledNpm) {
        $env:NPM_CONFIG_PREFIX = $nodeRoot
        $npmPrefixChanged = $true
    }
}

$releaseArguments = @(
    'run', './cmd/release',
    '-component', $Component,
    '-target-os', $TargetOS,
    '-architecture', $Architecture
)
if ($SkipNpmInstall) {
    $releaseArguments += '-skip-npm-install'
}

$previousGoCache = $env:GOCACHE
$env:GOCACHE = Join-Path $projectRoot '.cache\go-build'
Write-Host ''
Write-Host ('Building: {0} / {1} / {2}' -f $Component, $TargetOS, $Architecture) -ForegroundColor Green
Push-Location $projectRoot
try {
    & $goExecutable $releaseArguments
    if ($LASTEXITCODE -ne 0) {
        throw ('Release build failed with exit code {0}.' -f $LASTEXITCODE)
    }
} finally {
    Pop-Location
    $env:GOCACHE = $previousGoCache
    if ($npmPrefixChanged) {
        $env:NPM_CONFIG_PREFIX = $previousNpmPrefix
    }
}

$outputPath = Join-Path $projectRoot ('dist\{0}\{1}' -f $TargetOS, $Architecture)
Write-Host ('Release created at {0}' -f $outputPath) -ForegroundColor Green
