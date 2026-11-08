<#
.SYNOPSIS
Starts, stops, restarts, or reports the local Backend and Admin Web development services.

.DESCRIPTION
The Backend is built to .cache/dev/zentrola.exe and managed with its own start/stop/status
commands, reading the repository root .env. The Admin Web runs the Vite dev server in the
background on 127.0.0.1:9528 and proxies API requests to the Backend started here.
PostgreSQL and Redis are not managed by this script.

.EXAMPLE
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/Dev.ps1 start

.EXAMPLE
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/Dev.ps1 start -BackendPort 19527 -WebPort 19528
#>
[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('start', 'stop', 'restart', 'status')]
    [string]$Action = 'status',

    # 0 表示使用 .env 中 HTTP_ADDR 的端口。
    [ValidateRange(0, 65535)]
    [int]$BackendPort = 0,

    [ValidateRange(1, 65535)]
    [int]$WebPort = 9528,

    [int]$TimeoutSeconds = 60
)

$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent $PSScriptRoot
$devDir = Join-Path $projectRoot '.cache\dev'
$backendExe = Join-Path $devDir 'zentrola.exe'
$webDir = Join-Path $projectRoot 'web'
$webPidFile = Join-Path $devDir 'web.pid'
$webLog = Join-Path $devDir 'web.log'
$webUrl = "http://127.0.0.1:$WebPort/"

function Invoke-Backend([string[]]$Arguments) {
    # 后端以仓库根目录为工作目录，读取根目录 .env。
    # Windows PowerShell 5.1 在 Stop 模式下会把原生命令的 stderr 当作异常，这里按退出码判断。
    $ErrorActionPreference = 'Continue'
    Push-Location $projectRoot
    try {
        $output = & $backendExe @Arguments 2>&1 | ForEach-Object { "$_" }
        return [pscustomobject]@{ ExitCode = $LASTEXITCODE; Output = ($output -join [Environment]::NewLine) }
    }
    finally {
        Pop-Location
    }
}

function Get-ConfiguredBackendPort {
    if ($BackendPort -gt 0) {
        return $BackendPort
    }
    $envFile = Join-Path $projectRoot '.env'
    if (Test-Path -LiteralPath $envFile) {
        $match = Select-String -LiteralPath $envFile -Pattern '^\s*HTTP_ADDR\s*=\s*\S*:(\d+)\s*$' | Select-Object -Last 1
        if ($match) {
            return [int]$match.Matches[0].Groups[1].Value
        }
    }
    return 9527
}

function Test-BackendRunning {
    if (!(Test-Path -LiteralPath $backendExe)) {
        return $false
    }
    $result = Invoke-Backend 'status'
    return $result.ExitCode -eq 0 -and $result.Output -match '(?m)^Status:\s*running'
}

function Get-BackendAddress {
    $result = Invoke-Backend 'status'
    $match = [regex]::Match($result.Output, '(?m)^Address:\s*(\S+)')
    if ($match.Success) {
        return $match.Groups[1].Value.TrimEnd('/')
    }
    return 'http://127.0.0.1:9527'
}

function Wait-Http([string]$Url, [string]$Name) {
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        # 不经过系统代理：Invoke-WebRequest 在 Windows PowerShell 5.1 中每次都会探测代理，耗时数秒。
        $request = [System.Net.HttpWebRequest]::Create($Url)
        $request.Proxy = $null
        $request.Timeout = 3000
        try {
            $response = $request.GetResponse()
            $status = [int]$response.StatusCode
            $response.Close()
            if ($status -eq 200) {
                return
            }
        }
        catch {
            # 服务尚未就绪，稍后重试；非 2xx 响应也要释放连接。
            # PowerShell 会把 .NET 方法异常包装为 MethodInvocationException。
            $webError = $_.Exception
            if ($webError.InnerException) {
                $webError = $webError.InnerException
            }
            if ($webError -is [System.Net.WebException] -and $webError.Response) {
                $webError.Response.Close()
            }
        }
        Start-Sleep -Milliseconds 500
    }
    throw "$Name did not become ready within $TimeoutSeconds seconds: $Url"
}

function Start-Backend {
    if (Test-BackendRunning) {
        Write-Host "Backend is already running: $(Get-BackendAddress)"
        return
    }
    $port = Get-ConfiguredBackendPort
    if (Test-PortListening $port) {
        throw "Port $port is already in use by another process (for example a Backend started from an IDE); stop it or use -BackendPort."
    }
    # 运行中的 exe 在 Windows 上被锁定，只在未运行时重新编译。
    New-Item -ItemType Directory -Force -Path $devDir | Out-Null
    Write-Host 'Building Backend...'
    Push-Location $projectRoot
    try {
        & go build -o $backendExe ./cmd/server
        if ($LASTEXITCODE -ne 0) {
            throw 'Backend build failed.'
        }
    }
    finally {
        Pop-Location
    }
    $arguments = @('start')
    if ($BackendPort -gt 0) {
        $arguments += @('--port', "$BackendPort")
    }
    $result = Invoke-Backend $arguments
    if ($result.ExitCode -ne 0) {
        throw "Backend failed to start:$([Environment]::NewLine)$($result.Output)"
    }
    $address = Get-BackendAddress
    Wait-Http "$address/health/ready" 'Backend'
    Write-Host "Backend is ready: $address"
}

function Stop-Backend {
    if (!(Test-BackendRunning)) {
        Write-Host 'Backend is not running.'
        return
    }
    $result = Invoke-Backend 'stop'
    if ($result.ExitCode -ne 0) {
        throw "Backend failed to stop:$([Environment]::NewLine)$($result.Output)"
    }
    Write-Host 'Backend stopped.'
}

function Get-WebProcess {
    if (!(Test-Path -LiteralPath $webPidFile)) {
        return $null
    }
    $processId = 0
    if (![int]::TryParse((Get-Content -LiteralPath $webPidFile -Raw).Trim(), [ref]$processId)) {
        return $null
    }
    # 校验命令行，避免 PID 被系统复用后误停其他进程。
    $process = Get-CimInstance Win32_Process -Filter "ProcessId = $processId" -ErrorAction SilentlyContinue
    if ($process -and $process.CommandLine -like '*vite*') {
        return $process
    }
    return $null
}

function Test-PortListening([int]$Port) {
    # Get-NetTCPConnection 每次需数秒，直接读取本机 TCP 监听表。
    $listeners = [System.Net.NetworkInformation.IPGlobalProperties]::GetIPGlobalProperties().GetActiveTcpListeners()
    return @($listeners | Where-Object { $_.Port -eq $Port }).Count -gt 0
}

function Start-Web {
    if (Get-WebProcess) {
        Write-Host "Admin Web is already running: $webUrl"
        return
    }
    if (Test-PortListening $WebPort) {
        throw "Port $WebPort is already in use by another process; stop it before starting the Admin Web."
    }
    $vite = Join-Path $webDir 'node_modules\vite\bin\vite.js'
    if (!(Test-Path -LiteralPath $vite)) {
        throw 'Admin Web dependencies are missing; run "npm ci" in the web directory first.'
    }
    New-Item -ItemType Directory -Force -Path $devDir | Out-Null
    # Vite 代理指向本脚本启动的后端；后端未运行时按配置端口转发。
    if (Test-BackendRunning) {
        $env:ZENTROLA_API_TARGET = Get-BackendAddress
    }
    else {
        $env:ZENTROLA_API_TARGET = "http://127.0.0.1:$(Get-ConfiguredBackendPort)"
    }
    # 由 cmd 负责日志重定向：Start-Process 自身重定向时子进程会继承调用方的输出管道，
    # 调用方通过管道读取本脚本输出时会一直等到 Vite 退出。
    $viteCommand = "node node_modules\vite\bin\vite.js --host 127.0.0.1 --port $WebPort --strictPort > `"$webLog`" 2>&1"
    $process = Start-Process -FilePath $env:ComSpec -ArgumentList @('/d', '/c', "`"$viteCommand`"") `
        -WorkingDirectory $webDir -WindowStyle Hidden -PassThru
    Set-Content -LiteralPath $webPidFile -Value $process.Id -Encoding ascii
    try {
        Wait-Http $webUrl 'Admin Web'
    }
    catch {
        Stop-Web | Out-Null
        throw "$($_.Exception.Message)$([Environment]::NewLine)See $webLog."
    }
    Write-Host "Admin Web is ready: $webUrl"
}

function Stop-Web {
    $process = Get-WebProcess
    if (!$process) {
        Remove-Item -LiteralPath $webPidFile -ErrorAction SilentlyContinue
        Write-Host 'Admin Web is not running.'
        return
    }
    # 连同 cmd 下的 node 子进程一起结束。
    $ErrorActionPreference = 'Continue'
    & taskkill.exe /PID $process.ProcessId /T /F 2>&1 | Out-Null
    $ErrorActionPreference = 'Stop'
    Wait-Process -Id $process.ProcessId -Timeout 10 -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $webPidFile -ErrorAction SilentlyContinue
    Write-Host 'Admin Web stopped.'
}

function Show-Status {
    if (Test-BackendRunning) {
        Write-Host "Backend:   running  $(Get-BackendAddress)"
    }
    elseif (Test-PortListening (Get-ConfiguredBackendPort)) {
        Write-Host "Backend:   port $(Get-ConfiguredBackendPort) is in use by a process not started by this script"
    }
    else {
        Write-Host 'Backend:   stopped'
    }
    $web = Get-WebProcess
    if ($web) {
        Write-Host "Admin Web: running  $webUrl  (PID $($web.ProcessId))"
    }
    elseif (Test-PortListening $WebPort) {
        Write-Host "Admin Web: port $WebPort is in use by a process not started by this script"
    }
    else {
        Write-Host 'Admin Web: stopped'
    }
}

try {
    switch ($Action) {
        'start' {
            Start-Backend
            Start-Web
        }
        'stop' {
            Stop-Web
            Stop-Backend
        }
        'restart' {
            Stop-Web
            Stop-Backend
            Start-Backend
            Start-Web
        }
        'status' {
            Show-Status
        }
    }
}
catch {
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}
