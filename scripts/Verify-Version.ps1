[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent $PSScriptRoot
$version = (Get-Content -LiteralPath (Join-Path $projectRoot 'VERSION') -Raw).Trim()

if ($version -notmatch '^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$') {
    throw "VERSION contains an invalid semantic version: $version"
}

$envExample = Get-Content -LiteralPath (Join-Path $projectRoot '.env.example') -Raw
if ($envExample -match '(?m)^APP_VERSION=') {
    throw '.env.example must not duplicate the release version'
}

$dockerfile = Get-Content -LiteralPath (Join-Path $projectRoot 'Dockerfile') -Raw
$compose = Get-Content -LiteralPath (Join-Path $projectRoot 'compose.yaml') -Raw

Push-Location $projectRoot
try {
    $webHasVersion = (& node -p "const p=require('./web/package.json'),l=require('./web/package-lock.json'); [p,l,l.packages['']].some(x=>Object.hasOwn(x,'version'))").Trim()
    if ($LASTEXITCODE -ne 0) {
        throw 'Unable to read private web package metadata with Node.js'
    }
}
finally {
    Pop-Location
}
if ($webHasVersion -ne 'false') {
    throw 'Private web package metadata must not duplicate the application version'
}

if ($dockerfile -notmatch '(?m)^ARG APP_VERSION\r?$') {
    throw 'Dockerfile must declare APP_VERSION as a build argument without a default'
}

$composeVersions = [regex]::Matches($compose, '(?m)^\s*image:\s*longjianghu/zentrola:([^\s#]+)\s*$')
if ($composeVersions.Count -eq 0) {
    throw 'compose.yaml does not declare a longjianghu/zentrola image'
}
# Compose 固定跟随 latest；应用版本仅由 VERSION 提供。
foreach ($match in $composeVersions) {
    if ($match.Groups[1].Value -ne 'latest') {
        throw "compose.yaml image tag is '$($match.Groups[1].Value)', expected 'latest'"
    }
}

Write-Host "Version metadata is consistent: $version"
