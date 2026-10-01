#requires -Version 5.1
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$outputFull = Join-Path $projectRoot 'third_party_licenses'

$staging = Join-Path $projectRoot '.cache\third-party-licenses-staging'
if (Test-Path -LiteralPath $staging) {
    Remove-Item -LiteralPath $staging -Recurse -Force
}
New-Item -ItemType Directory -Path $staging | Out-Null

function ConvertTo-SafeName {
    param([Parameter(Mandatory = $true)][string]$Value)
    return $Value -replace '[^A-Za-z0-9._@+-]', '_'
}

function Copy-LicenseFiles {
    param(
        [Parameter(Mandatory = $true)][string]$Source,
        [Parameter(Mandatory = $true)][string]$Destination
    )
    $licenses = @(Get-ChildItem -LiteralPath $Source -File | Where-Object {
        $_.Name -match '^(LICENSE|LICENCE|COPYING|NOTICE)(\.(txt|md|libyaml))?$'
    })
    if (!$licenses) {
        return $false
    }
    New-Item -ItemType Directory -Path $Destination -Force | Out-Null
    foreach ($license in $licenses) {
        Copy-Item -LiteralPath $license.FullName -Destination (Join-Path $Destination $license.Name)
    }
    return $true
}

Push-Location $projectRoot
try {
    $goModules = & go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}{{end}}' ./cmd/server ./cmd/web |
        Where-Object { $_ } |
        Sort-Object -Unique
    if ($LASTEXITCODE -ne 0) {
        throw 'Unable to enumerate Go runtime dependencies.'
    }
    foreach ($entry in $goModules) {
        $parts = $entry -split '\|', 3
        $destination = Join-Path $staging ('go\{0}@{1}' -f (ConvertTo-SafeName $parts[0]), (ConvertTo-SafeName $parts[1]))
        if (!(Copy-LicenseFiles -Source $parts[2] -Destination $destination)) {
            throw ('No license file found for Go module {0}@{1}.' -f $parts[0], $parts[1])
        }
    }

    $nodeScript = @'
const fs = require('fs')
const path = require('path')
const lock = require('./web/package-lock.json')
for (const [packagePath, metadata] of Object.entries(lock.packages)) {
  if (!packagePath || metadata.dev === true) continue
  const name = packagePath.replace(/^node_modules\//, '')
  console.log([name, metadata.version, metadata.license || '', path.join('web', packagePath)].join('|'))
}
'@
    $npmPackages = & node -e $nodeScript
    if ($LASTEXITCODE -ne 0) {
        throw 'Unable to enumerate npm runtime dependencies.'
    }
    foreach ($entry in $npmPackages) {
        $parts = $entry -split '\|', 4
        $packageDirectory = 'npm\{0}@{1}' -f (ConvertTo-SafeName $parts[0]), (ConvertTo-SafeName $parts[1])
        $destination = Join-Path $staging $packageDirectory
        if (!(Copy-LicenseFiles -Source (Join-Path $projectRoot $parts[3]) -Destination $destination)) {
            $override = Join-Path $projectRoot (Join-Path 'scripts\third_party_license_overrides' $packageDirectory)
            if (!(Test-Path -LiteralPath $override) -or !(Copy-LicenseFiles -Source $override -Destination $destination)) {
                throw ('No license text found for npm package {0}@{1} (declared license: {2}).' -f $parts[0], $parts[1], $parts[2])
            }
        }
    }
} finally {
    Pop-Location
}

if (Test-Path -LiteralPath $outputFull) {
    Remove-Item -LiteralPath $outputFull -Recurse -Force
}
Move-Item -LiteralPath $staging -Destination $outputFull
Write-Host ('Third-party license files synchronized at {0}' -f $outputFull)
