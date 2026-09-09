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
$useChinese = [System.Globalization.CultureInfo]::CurrentUICulture.Name.StartsWith('zh', [System.StringComparison]::OrdinalIgnoreCase)
if ($useChinese) {
    $messages = @{
        SelectComponent = '选择发布内容'
        SelectOS = '选择目标操作系统'
        SelectArchitecture = '选择目标 CPU 架构'
        SelectRange = '请选择 1-{0}'
        InvalidSelection = '输入无效，请重新选择。'
        GoNotFound = '未找到 Go，请先安装 Go 并确保 go 命令已加入 PATH。'
        NodeNotFound = '构建 Admin Web 需要 Node.js 和 npm，请先安装并加入 PATH。'
        Building = '开始构建：{0} / {1} / {2}'
        BuildFailed = '发布构建失败，退出码：{0}'
        ReleaseCreated = '构建完成：{0}'
    }
} else {
    $messages = @{
        SelectComponent = 'Select release component'
        SelectOS = 'Select target operating system'
        SelectArchitecture = 'Select target CPU architecture'
        SelectRange = 'Select 1-{0}'
        InvalidSelection = 'Invalid selection. Try again.'
        GoNotFound = 'Go was not found. Install Go and add it to PATH.'
        NodeNotFound = 'Building Admin Web requires Node.js and npm in PATH.'
        Building = 'Building: {0} / {1} / {2}'
        BuildFailed = 'Release build failed with exit code {0}.'
        ReleaseCreated = 'Release created at {0}'
    }
}

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
        $selection = Read-Host ($messages.SelectRange -f $Options.Count)
        $parsed = 0
        if ([int]::TryParse($selection, [ref]$parsed) -and $parsed -ge 1 -and $parsed -le $Options.Count) {
            return $Options[$parsed - 1]
        }
        Write-Host $messages.InvalidSelection -ForegroundColor Yellow
    }
}

if (!$Component) {
    $Component = Read-ReleaseChoice $messages.SelectComponent @('backend', 'web', 'all')
}
if (!$TargetOS) {
    $TargetOS = Read-ReleaseChoice $messages.SelectOS @('windows', 'macos', 'linux')
}
if (!$Architecture) {
    $architectures = @('amd64', 'arm64', '386')
    if ($TargetOS -eq 'macos') {
        $architectures = @('amd64', 'arm64')
    }
    $Architecture = Read-ReleaseChoice $messages.SelectArchitecture $architectures
}

$projectRoot = Split-Path -Parent $PSScriptRoot
$goCommand = Get-Command go -ErrorAction SilentlyContinue
if (!$goCommand) {
    throw $messages.GoNotFound
}
$goExecutable = $goCommand.Source

$previousNpmPrefix = $env:NPM_CONFIG_PREFIX
$npmPrefixChanged = $false
if ($Component -ne 'backend') {
    $npmCommand = Get-Command npm.cmd -ErrorAction SilentlyContinue
    $nodeCommand = Get-Command node.exe -ErrorAction SilentlyContinue
    if (!$npmCommand -or !$nodeCommand) {
        throw $messages.NodeNotFound
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
Write-Host ($messages.Building -f $Component, $TargetOS, $Architecture) -ForegroundColor Green
Push-Location $projectRoot
try {
    & $goExecutable $releaseArguments
    if ($LASTEXITCODE -ne 0) {
        throw ($messages.BuildFailed -f $LASTEXITCODE)
    }
} finally {
    Pop-Location
    $env:GOCACHE = $previousGoCache
    if ($npmPrefixChanged) {
        $env:NPM_CONFIG_PREFIX = $previousNpmPrefix
    }
}

$outputPath = Join-Path $projectRoot ('dist\{0}\{1}' -f $TargetOS, $Architecture)
Write-Host ($messages.ReleaseCreated -f $outputPath) -ForegroundColor Green
