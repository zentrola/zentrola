#requires -Version 7.4
# 仅在显式执行时调用已配置的真实 DeepSeek Resource，最多四次短文本推理。
# 不读取/保存上游密钥；管理员密码和临时 Virtual Key 只在本进程使用。
[CmdletBinding()]
param(
    [Parameter(Mandatory)][Security.SecureString]$AdminPassword,
    [string]$AdminUsername = 'admin',
    [switch]$VerifyUsage
)
$ErrorActionPreference = 'Stop'
$base = 'http://127.0.0.1:8080'
$adminHeaders = @{}
$member = $null
$group = $null
$key = $null
$model = $null
$checks = [Collections.Generic.List[object]]::new()
$expectedUsage = [Collections.Generic.List[object]]::new()

function Admin-Request([string]$Method, [string]$Path, $Body = $null) {
    $argsForRequest = @{Method=$Method; Uri="$base/api/v1$Path"; Headers=$adminHeaders; TimeoutSec=30; MaximumRedirection=0}
    if ($null -ne $Body) { $argsForRequest.ContentType='application/json'; $argsForRequest.Body=ConvertTo-Json -InputObject $Body -Depth 30 -Compress }
    try { return (Invoke-RestMethod @argsForRequest).data }
    catch { throw "Management request failed: $Method $Path (response omitted)" }
}
function Check([string]$Name, [bool]$OK, $Detail) {
    $checks.Add([pscustomobject]@{name=$Name;passed=$OK;detail=$Detail})
    Write-Output "$Name : $OK"
    if (!$OK) { throw "DeepSeek check failed: $Name" }
}
function Gateway-Request($Body, [string]$Path='/v1/messages') {
    $handler = [Net.Http.HttpClientHandler]::new()
    $handler.AllowAutoRedirect=$false
    $handler.UseProxy=$false
    $client = [Net.Http.HttpClient]::new($handler)
    $client.Timeout=[TimeSpan]::FromSeconds(120)
    $request=[Net.Http.HttpRequestMessage]::new([Net.Http.HttpMethod]::Post, "$base/anthropic$Path")
    $request.Headers.Add('x-api-key',$key.key)
    $request.Headers.Add('anthropic-version','2023-06-01')
    $request.Content=[Net.Http.StringContent]::new((ConvertTo-Json -InputObject $Body -Depth 30 -Compress),[Text.Encoding]::UTF8,'application/json')
    $response=$null
    $deadline=[Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(120))
    $watch=[Diagnostics.Stopwatch]::StartNew()
    try {
        $response=$client.SendAsync($request,[Net.Http.HttpCompletionOption]::ResponseHeadersRead,$deadline.Token).GetAwaiter().GetResult()
        $status=[int]$response.StatusCode
        $requestId=@($response.Headers.GetValues('X-Request-ID'))[0]
        if ($Body.stream -and $status -eq 200) {
            $stream=$response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
            $reader=[IO.StreamReader]::new($stream)
            $events=[Collections.Generic.List[string]]::new()
            $firstDeltaMS=$null
            $textParts=[Text.StringBuilder]::new()
            $usageValues=@{}
            try {
                while ($null -ne ($line=$reader.ReadLineAsync($deadline.Token).GetAwaiter().GetResult())) {
                    if ($line.StartsWith('event: ')) { $events.Add($line.Substring(7)) }
                    if ($line.StartsWith('data: ')) {
                        $event=$line.Substring(6)|ConvertFrom-Json
                        $usage=$null
                        if ($event.type -eq 'message_start') { $usage=$event.message.usage }
                        if ($event.type -eq 'message_delta') { $usage=$event.usage }
                        if ($null -ne $usage) { foreach ($prop in $usage.PSObject.Properties) { $usageValues[$prop.Name]=$prop.Value } }
                        if ($event.type -eq 'content_block_delta' -and $event.delta.type -eq 'text_delta') {
                            if ($null -eq $firstDeltaMS) { $firstDeltaMS=$watch.ElapsedMilliseconds }
                            $null=$textParts.Append($event.delta.text)
                        }
                    }
                }
            } finally { $reader.Dispose() }
            return @{status=$status;events=$events.ToArray();firstDeltaMS=$firstDeltaMS;elapsedMS=$watch.ElapsedMilliseconds;text=$textParts.ToString();requestId=$requestId;usage=$usageValues}
        }
        $raw=$response.Content.ReadAsStringAsync($deadline.Token).GetAwaiter().GetResult()
        $data=$null
        try { $data=$raw|ConvertFrom-Json } catch { }
        return @{status=$status;data=$data;elapsedMS=$watch.ElapsedMilliseconds;requestId=$requestId;usage=$data.usage}
    } finally {
        if ($response) { $response.Dispose() }
        $request.Dispose()
        $client.Dispose()
        $deadline.Dispose()
    }
}

try {
    $password=[Net.NetworkCredential]::new('', $AdminPassword).Password
    $login=Admin-Request POST '/auth/login' @{username=$AdminUsername;password=$password}
    $password=$null
    $adminHeaders.Authorization="Bearer $($login.token)"
    $login=$null
    $models=Admin-Request GET '/models'
    $model=@($models.items|Where-Object code -eq 'deepseek-v4-flash')[0]
    if (!$model) { throw 'Configure the DeepSeek provider and model mappings in the database first' }
    $stamp=[DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds().ToString()
    $member=Admin-Request POST '/members' @{name="DeepSeek 验证 $stamp";remark='真实接口验证；执行结束后停用'}
    $group=Admin-Request POST '/groups' @{code="deepseek-check-$stamp";name='DeepSeek 接入验证';remark='执行结束后清除授权'}
    $null=Admin-Request PUT "/groups/$($group.id)/members/$($member.id)"
    $null=Admin-Request PUT "/groups/$($group.id)/models/$($model.id)"
    $key=Admin-Request POST "/members/$($member.id)/keys" @{name='DeepSeek temporary test';expiresAt=[DateTime]::UtcNow.AddMinutes(15).ToString('o')}

    $body=@{model='deepseek-v4-flash';max_tokens=128;thinking=@{type='disabled'};messages=@(@{role='user';content='Reply with only ZENTROLA_OK.'})}
    $result=Gateway-Request $body
    Check 'messages' ($result.status -eq 200 -and $result.data.type -eq 'message' -and $result.data.model -like 'deepseek*' -and ($result.data.content.text -join '') -match 'ZENTROLA_OK') @{status=$result.status;model=$result.data.model;elapsedMS=$result.elapsedMS}
    $expectedUsage.Add($result)

    $body.stream=$true
    $body.messages=@(@{role='user';content='List the integers from 1 to 30, separated by spaces. Output nothing else.'})
    $result=Gateway-Request $body
    Check 'SSE' ($result.status -eq 200 -and $result.events -contains 'message_start' -and $result.events -contains 'message_stop' -and $result.events -contains 'content_block_delta' -and $null -ne $result.firstDeltaMS) @{status=$result.status;firstDeltaMS=$result.firstDeltaMS;elapsedMS=$result.elapsedMS;eventCount=$result.events.Count}
    $expectedUsage.Add($result)

    $tools=@(@{name='zentrola_sum';description='Add two integers.';input_schema=@{type='object';properties=@{a=@{type='integer'};b=@{type='integer'}};required=@('a','b')}})
    $messages=@(@{role='user';content='Use zentrola_sum to add 2 and 3. After the tool returns, reply with the result.'})
    $body=@{model='deepseek-v4-flash';max_tokens=256;thinking=@{type='disabled'};tools=$tools;tool_choice=@{type='tool';name='zentrola_sum'};messages=$messages}
    $first=Gateway-Request $body
    $toolUse=@($first.data.content|Where-Object type -eq 'tool_use')
    Check 'tool_use' ($first.status -eq 200 -and $first.data.stop_reason -eq 'tool_use' -and $toolUse.Count -eq 1 -and $toolUse[0].name -eq 'zentrola_sum' -and $toolUse[0].input.a -eq 2 -and $toolUse[0].input.b -eq 3) @{status=$first.status;stopReason=$first.data.stop_reason}
    $body.messages=$messages+@(@{role='assistant';content=$first.data.content},@{role='user';content=@(@{type='tool_result';tool_use_id=$toolUse[0].id;content='5'})})
    $body.tool_choice=@{type='none'}
    $second=Gateway-Request $body
    Check 'tool_result' ($second.status -eq 200 -and $second.data.stop_reason -eq 'end_turn' -and ($second.data.content.text -join '') -match '5') @{status=$second.status;stopReason=$second.data.stop_reason}
    $expectedUsage.Add($first)
    $expectedUsage.Add($second)

    $count=Gateway-Request @{model='deepseek-v4-flash';messages=@(@{role='user';content='hello'})} '/v1/messages/count_tokens'
    # 上游能力单独报告；不把平台估算值当成官方计数。
    $checks.Add([pscustomobject]@{name='count_tokens';passed=($count.status -eq 200 -and $count.data.input_tokens -gt 0);detail=@{status=$count.status;inputTokens=$count.data.input_tokens}})
    Write-Output "count_tokens HTTP: $($count.status)"

    $denied=Gateway-Request @{model='deepseek-v4-pro';max_tokens=16;messages=@(@{role='user';content='hello'})}
    Check 'unauthorized_model' ($denied.status -eq 403) @{status=$denied.status}
    $null=Admin-Request POST "/access-keys/$($key.id)/revoke"
    $revoked=Gateway-Request @{model='deepseek-v4-flash';max_tokens=16;messages=@(@{role='user';content='hello'})}
    Check 'revoked_key' ($revoked.status -eq 401) @{status=$revoked.status}
    if ($VerifyUsage) {
        $pollUntil=[DateTime]::UtcNow.AddSeconds(5)
        do {
            $usageRows=Admin-Request GET "/usage?memberId=$($member.id)"
            if ($usageRows.items.Count -ge 5) { break }
            Start-Sleep -Milliseconds 100
        } while ([DateTime]::UtcNow -lt $pollUntil)
        Check 'usage_request_count' ($usageRows.items.Count -eq 5) @{requests=$usageRows.items.Count;expected=5}
        $providers=Admin-Request GET '/providers'
        $provider=@($providers.items|Where-Object code -eq 'deepseek-official')[0]
        $resources=Admin-Request GET '/resources'
        $resource=@($resources.items|Where-Object { $_.providerId -eq $provider.id -and $_.status -eq 'ACTIVE' })[0]
        foreach ($expected in $expectedUsage) {
            $record=@($usageRows.items|Where-Object requestId -eq $expected.requestId)[0]
            $correct=$record.status -eq 'SUCCESS' -and $record.principalId -eq $member.id -and $record.modelId -eq $model.id -and $record.providerId -eq $provider.id -and $record.resourceId -eq $resource.id -and $record.attemptNo -eq 1
            foreach ($pair in @(@('input_tokens','inputTokens'),@('output_tokens','outputTokens'),@('cache_read_input_tokens','cachedInputTokens'))) {
                if ($record.($pair[1]) -ne $expected.usage.($pair[0])) { $correct=$false }
            }
            Check 'usage_attribution_and_tokens' $correct @{requestId=$expected.requestId;inputTokens=$record.inputTokens;outputTokens=$record.outputTokens;cachedInputTokens=$record.cachedInputTokens}
        }
        $deniedRow=@($usageRows.items|Where-Object requestId -eq $denied.requestId)[0]
        Check 'denied_without_attempt' ($deniedRow.status -eq 'FAILED' -and $null -eq $deniedRow.usageId) @{requestId=$denied.requestId}
    }
} finally {
    $cleanupFailed=$false
    foreach ($action in @(
        { if ($key) { $null=Admin-Request POST "/access-keys/$($key.id)/revoke" } },
        { if ($group -and $model) { $null=Admin-Request DELETE "/groups/$($group.id)/models/$($model.id)" } },
        { if ($group -and $member) { $null=Admin-Request DELETE "/groups/$($group.id)/members/$($member.id)" } },
        { if ($member) { $null=Admin-Request PATCH "/members/$($member.id)/status" @{status='DISABLED'} } }
    )) {
        try { & $action } catch { $cleanupFailed=$true }
    }
    $checks.Add([pscustomobject]@{name='cleanup';passed=(!$cleanupFailed);detail='Temporary key revoked, member disabled, group permissions removed.'})
    $reportDir=Join-Path $PSScriptRoot '../.cache'
    $null=New-Item -ItemType Directory -Force -Path $reportDir
    [pscustomobject]@{at=[DateTime]::UtcNow.ToString('o');checks=$checks.ToArray()}|ConvertTo-Json -Depth 10|Set-Content (Join-Path $reportDir 'deepseek-live-results.json') -Encoding utf8
    $adminHeaders.Clear()
    $key=$null
    if ($cleanupFailed) { Write-Warning 'Temporary test access cleanup was incomplete; inspect operation logs.' }
}
