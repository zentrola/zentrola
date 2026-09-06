#requires -Version 7.4
# 从接口旁的 Swaggo 注释生成随二进制打包的文档；需要 Go 1.26+。
[CmdletBinding()]
param([string]$Go)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
if (!$Go) {
    $goCommand = Get-Command go -ErrorAction SilentlyContinue
    if ($goCommand) {
        $Go = $goCommand.Source
    } else {
        $Go = Join-Path $projectRoot '.cache/toolchain/go/bin/go.exe'
    }
}
if (!(Test-Path -LiteralPath $Go)) {
    throw 'Go executable not found; pass -Go with an absolute executable path.'
}

$previousPath = $env:PATH
$env:PATH = (Split-Path -Parent $Go) + [IO.Path]::PathSeparator + $env:PATH
Push-Location $projectRoot
try {
    & $Go tool swag init -d ./cmd/server,./internal/transport/http -g main.go --parseInternal --parseDependencyLevel 1 --parseFuncBody --outputTypes json -o ./internal/transport/http/apidocs
    if ($LASTEXITCODE -ne 0) { throw 'Swagger generation failed.' }
} finally {
    Pop-Location
    $env:PATH = $previousPath
}
