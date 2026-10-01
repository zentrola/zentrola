[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent $PSScriptRoot
$version = (Get-Content -LiteralPath (Join-Path $projectRoot 'VERSION') -Raw).Trim()

if ($version -notmatch '^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$') {
    throw "VERSION contains an invalid semantic version: $version"
}

$dockerfile = Get-Content -LiteralPath (Join-Path $projectRoot 'Dockerfile') -Raw
$compose = Get-Content -LiteralPath (Join-Path $projectRoot 'compose.yaml') -Raw

Push-Location $projectRoot
try {
    $packageVersion = (& node -p "require('./web/package.json').version").Trim()
    if ($LASTEXITCODE -ne 0) {
        throw 'Unable to read web/package.json with Node.js'
    }
    $packageLockVersion = (& node -p "require('./web/package-lock.json').version").Trim()
    if ($LASTEXITCODE -ne 0) {
        throw 'Unable to read web/package-lock.json with Node.js'
    }
    $packageLockRootVersion = (& node -p "require('./web/package-lock.json').packages[''].version").Trim()
    if ($LASTEXITCODE -ne 0) {
        throw 'Unable to read the root package version from web/package-lock.json with Node.js'
    }
}
finally {
    Pop-Location
}

$checks = [ordered]@{
    'web/package.json' = $packageVersion
    'web/package-lock.json' = $packageLockVersion
    'web/package-lock.json packages[""]' = $packageLockRootVersion
}

foreach ($entry in $checks.GetEnumerator()) {
    if ($entry.Value -ne $version) {
        throw "$($entry.Key) version is '$($entry.Value)', expected '$version'"
    }
}

if ($dockerfile -notmatch "(?m)^ARG APP_VERSION=$([regex]::Escape($version))\r?$") {
    throw "Dockerfile APP_VERSION does not match VERSION '$version'"
}

$composeVersions = [regex]::Matches($compose, '(?m)^\s*image:\s*longjianghu/zentrola:([^\s#]+)\s*$')
if ($composeVersions.Count -eq 0) {
    throw 'compose.yaml does not declare a longjianghu/zentrola image'
}
foreach ($match in $composeVersions) {
    if ($match.Groups[1].Value -ne $version) {
        throw "compose.yaml image version is '$($match.Groups[1].Value)', expected '$version'"
    }
}

Write-Host "Version metadata is consistent: $version"
