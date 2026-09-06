#requires -Version 7.4
# 显式执行后通过本地 Gateway 发起四次短 DeepSeek 推理；不读取上游密钥。
[CmdletBinding()]
param([Parameter(Mandatory)][Security.SecureString]$AdminPassword,[string]$AdminUsername='admin')
$ErrorActionPreference='Stop'
$base='http://127.0.0.1:8080'
$adminHeaders=@{}
$member=$null;$group=$null;$key=$null;$model=$null
$checks=[Collections.Generic.List[object]]::new()
$expected=[Collections.Generic.List[object]]::new()
function AdminCall($Method,$Path,$Body=$null){
    $args=@{Method=$Method;Uri="$base/api/v1$Path";Headers=$adminHeaders;TimeoutSec=30;MaximumRedirection=0}
    if ($null -ne $Body){$args.ContentType='application/json';$args.Body=ConvertTo-Json -InputObject $Body -Depth 30 -Compress}
    try { (Invoke-RestMethod @args).data } catch { throw "Management request failed: $Method $Path (response omitted)" }
}
function Check($Name,[bool]$Passed,$Detail){
    $checks.Add([pscustomobject]@{name=$Name;passed=$Passed;detail=$Detail})
    Write-Output "$Name : $Passed"
    if (!$Passed){throw "OpenAI compatibility check failed: $Name"}
}
function GatewayCall($Body=$null,[string]$Path='/v1/chat/completions'){
    $handler=[Net.Http.HttpClientHandler]::new();$handler.UseProxy=$false;$handler.AllowAutoRedirect=$false
    $client=[Net.Http.HttpClient]::new($handler)
    $method=[Net.Http.HttpMethod]::Post;if ($null -eq $Body){$method=[Net.Http.HttpMethod]::Get}
    $request=[Net.Http.HttpRequestMessage]::new($method,"$base$Path")
    $request.Headers.Add('Authorization',"Bearer $($key.key)")
    if ($Body){$request.Content=[Net.Http.StringContent]::new((ConvertTo-Json -InputObject $Body -Depth 30 -Compress),[Text.Encoding]::UTF8,'application/json')}
    $deadline=[Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(120));$response=$null
    $watch=[Diagnostics.Stopwatch]::StartNew()
    try {
        $response=$client.SendAsync($request,[Net.Http.HttpCompletionOption]::ResponseHeadersRead,$deadline.Token).GetAwaiter().GetResult()
        $status=[int]$response.StatusCode;$requestId=@($response.Headers.GetValues('X-Request-ID'))[0]
        if ($Body.stream -and $status -eq 200){
            $reader=[IO.StreamReader]::new($response.Content.ReadAsStreamAsync().GetAwaiter().GetResult())
            $done=$false;$chunks=0;$firstMS=$null;$usage=$null;$text=[Text.StringBuilder]::new()
            try {
                while ($null -ne ($line=$reader.ReadLineAsync($deadline.Token).GetAwaiter().GetResult())){
                    if (!$line.StartsWith('data:')){continue}
                    $data=$line.Substring(5).Trim()
                    if ($data -eq '[DONE]'){$done=$true;continue}
                    $chunk=$data|ConvertFrom-Json;$chunks++
                    if ($null -ne $chunk.usage){$usage=$chunk.usage}
                    foreach ($choice in $chunk.choices){if ($choice.delta.content){if ($null -eq $firstMS){$firstMS=$watch.ElapsedMilliseconds};$null=$text.Append($choice.delta.content)}}
                }
            } finally {$reader.Dispose()}
            return @{status=$status;requestId=$requestId;usage=$usage;done=$done;chunks=$chunks;firstDeltaMS=$firstMS;elapsedMS=$watch.ElapsedMilliseconds;text=$text.ToString()}
        }
        $raw=$response.Content.ReadAsStringAsync($deadline.Token).GetAwaiter().GetResult();$data=$raw|ConvertFrom-Json
        return @{status=$status;requestId=$requestId;data=$data;usage=$data.usage;elapsedMS=$watch.ElapsedMilliseconds}
    } finally {if ($response){$response.Dispose()};$request.Dispose();$client.Dispose();$deadline.Dispose()}
}
try {
    $login=AdminCall POST '/auth/login' @{username=$AdminUsername;password=[Net.NetworkCredential]::new('',$AdminPassword).Password}
    $adminHeaders.Authorization="Bearer $($login.token)";$login=$null
    $models=AdminCall GET '/models';$model=@($models.items|Where-Object code -eq 'deepseek-v4-flash')[0]
    if (!$model){throw 'Configure the DeepSeek provider and model mappings in the database first'}
    $providers=AdminCall GET '/providers';$provider=@($providers.items|Where-Object code -eq 'deepseek-official')[0]
    $resources=AdminCall GET '/resources';$resource=@($resources.items|Where-Object {$_.providerId -eq $provider.id -and $_.status -eq 'ACTIVE'})[0]
    if (!$resource){throw 'An ACTIVE DeepSeek resource is required'}
    $stamp=[DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds().ToString()
    $member=AdminCall POST '/members' @{name="OpenAI 验证 $stamp";remark='真实兼容接口验证；完成后停用'}
    $group=AdminCall POST '/groups' @{code="openai-check-$stamp";name='OpenAI 接入验证'}
    $null=AdminCall PUT "/groups/$($group.id)/members/$($member.id)"
    $null=AdminCall PUT "/groups/$($group.id)/models/$($model.id)"
    $key=AdminCall POST "/members/$($member.id)/keys" @{name='OpenAI temporary test';expiresAt=[DateTime]::UtcNow.AddMinutes(15).ToString('o')}
    $discovery=GatewayCall -Path '/v1/models'
    Check 'authorized_models' ($discovery.status -eq 200 -and $discovery.data.object -eq 'list' -and $discovery.data.data.Count -eq 1 -and $discovery.data.data[0].id -eq 'deepseek-v4-flash') @{status=$discovery.status;count=$discovery.data.data.Count}

    $body=@{model='deepseek-v4-flash';max_tokens=128;thinking=@{type='disabled'};messages=@(@{role='user';content='Reply with only ZENTROLA_OK.'})}
    $normal=GatewayCall $body
    Check 'chat_completions' ($normal.status -eq 200 -and $normal.data.object -eq 'chat.completion' -and $normal.data.choices[0].message.content -match 'ZENTROLA_OK') @{status=$normal.status;model=$normal.data.model}
    $expected.Add($normal)
    $body.stream=$true;$body.stream_options=@{include_usage=$true};$body.messages=@(@{role='user';content='List the integers 1 through 20 separated by spaces. Output nothing else.'})
    $stream=GatewayCall $body
    Check 'SSE' ($stream.status -eq 200 -and $stream.done -and $stream.chunks -gt 2 -and $null -ne $stream.firstDeltaMS) @{status=$stream.status;chunks=$stream.chunks;firstDeltaMS=$stream.firstDeltaMS;elapsedMS=$stream.elapsedMS}
    $expected.Add($stream)
    $tools=@(@{type='function';function=@{name='zentrola_sum';description='Add two integers.';parameters=@{type='object';properties=@{a=@{type='integer'};b=@{type='integer'}};required=@('a','b')}}})
    $messages=@(@{role='user';content='Use zentrola_sum to add 2 and 3. After the tool returns, reply with the result.'})
    $body=@{model='deepseek-v4-flash';max_tokens=256;thinking=@{type='disabled'};messages=$messages;tools=$tools;tool_choice=@{type='function';function=@{name='zentrola_sum'}}}
    $first=GatewayCall $body;$calls=@($first.data.choices[0].message.tool_calls)
    $args=$null;if ($calls.Count -eq 1){$args=$calls[0].function.arguments|ConvertFrom-Json}
    Check 'tool_calls' ($first.status -eq 200 -and $first.data.choices[0].finish_reason -eq 'tool_calls' -and $calls.Count -eq 1 -and $calls[0].function.name -eq 'zentrola_sum' -and $args.a -eq 2 -and $args.b -eq 3) @{status=$first.status;finishReason=$first.data.choices[0].finish_reason}
    $expected.Add($first)
    $body.messages=$messages+@($first.data.choices[0].message,@{role='tool';tool_call_id=$calls[0].id;content='5'});$body.tool_choice='none'
    $second=GatewayCall $body
    Check 'tool_result' ($second.status -eq 200 -and $second.data.choices[0].finish_reason -eq 'stop' -and $second.data.choices[0].message.content -match '5') @{status=$second.status;finishReason=$second.data.choices[0].finish_reason}
    $expected.Add($second)

    $denied=GatewayCall @{model='deepseek-v4-pro';max_tokens=16;messages=@(@{role='user';content='hello'})}
    Check 'unauthorized_model' ($denied.status -eq 403 -and $denied.data.error.code -eq 'MODEL_PERMISSION_DENIED') @{status=$denied.status}
    $null=AdminCall POST "/access-keys/$($key.id)/revoke"
    $revoked=GatewayCall -Path '/v1/models'
    Check 'revoked_key' ($revoked.status -eq 401 -and $revoked.data.error.code -eq 'UNAUTHENTICATED') @{status=$revoked.status}
    $until=[DateTime]::UtcNow.AddSeconds(5)
    do {$rows=AdminCall GET "/usage?memberId=$($member.id)";if ($rows.items.Count -ge 5){break};Start-Sleep -Milliseconds 100} while ([DateTime]::UtcNow -lt $until)
    Check 'usage_count' ($rows.items.Count -eq 5) @{requests=$rows.items.Count}
    foreach ($result in $expected){
        $record=@($rows.items|Where-Object requestId -eq $result.requestId)[0]
        $cached=$result.usage.prompt_cache_hit_tokens;if ($null -eq $cached){$cached=$result.usage.prompt_tokens_details.cached_tokens}
        $ok=$record.clientProtocol -eq 'OPENAI' -and $record.status -eq 'SUCCESS' -and $record.principalId -eq $member.id -and $record.modelId -eq $model.id -and $record.providerId -eq $provider.id -and $record.resourceId -eq $resource.id -and $record.attemptNo -eq 1 -and $record.inputTokens -eq $result.usage.prompt_tokens -and $record.outputTokens -eq $result.usage.completion_tokens -and $record.cachedInputTokens -eq $cached
        Check 'usage_attribution' $ok @{requestId=$result.requestId;inputTokens=$record.inputTokens;outputTokens=$record.outputTokens;cachedInputTokens=$record.cachedInputTokens}
    }
    $rejection=@($rows.items|Where-Object requestId -eq $denied.requestId)[0]
    Check 'denied_without_attempt' ($rejection.clientProtocol -eq 'OPENAI' -and $rejection.status -eq 'FAILED' -and $null -eq $rejection.usageId) @{requestId=$denied.requestId}
} finally {
    $cleanupFailed=$false
    foreach ($action in @(
        {if ($key){$null=AdminCall POST "/access-keys/$($key.id)/revoke"}},
        {if ($group -and $model){$null=AdminCall DELETE "/groups/$($group.id)/models/$($model.id)"}},
        {if ($group -and $member){$null=AdminCall DELETE "/groups/$($group.id)/members/$($member.id)"}},
        {if ($member){$null=AdminCall PATCH "/members/$($member.id)/status" @{status='DISABLED'}}}
    )){try{& $action}catch{$cleanupFailed=$true}}
    $checks.Add([pscustomobject]@{name='cleanup';passed=(!$cleanupFailed)})
    $reportDir=Join-Path $PSScriptRoot '../.cache';$null=New-Item -ItemType Directory -Force -Path $reportDir
    [pscustomobject]@{at=[DateTime]::UtcNow.ToString('o');checks=$checks.ToArray()}|ConvertTo-Json -Depth 8|Set-Content (Join-Path $reportDir 'openai-live-results.json') -Encoding utf8
    $adminHeaders.Clear();$key=$null
    if ($cleanupFailed){Write-Warning 'Temporary test access cleanup was incomplete.'}
}
