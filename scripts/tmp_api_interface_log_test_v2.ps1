param()

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$env:EMAIL_AUTH_CODE = 'dummy'

$BaseUrl = 'http://127.0.0.1:8080'
$CoreServices = @('gateway', 'auth', 'user', 'relation', 'msg')
$GatewayOnly = @('gateway')
$script:Results = New-Object System.Collections.Generic.List[object]

function New-TraceId {
	return [guid]::NewGuid().ToString()
}

function To-JsonText {
	param([object]$Value)
	if ($null -eq $Value) {
		return $null
	}
	return ($Value | ConvertTo-Json -Compress -Depth 20)
}

function Short-Text {
	param(
		[string]$Text,
		[int]$Max = 320
	)
	if ([string]::IsNullOrEmpty($Text)) {
		return $Text
	}
	if ($Text.Length -le $Max) {
		return $Text
	}
	return $Text.Substring(0, $Max) + '...'
}

function Get-PropValue {
	param(
		[object]$Object,
		[string[]]$Names
	)
	if ($null -eq $Object) {
		return $null
	}
	foreach ($name in $Names) {
		if ($Object.PSObject.Properties[$name]) {
			$value = $Object.$name
			if ($null -ne $value -and "$value" -ne '') {
				return $value
			}
		}
	}
	return $null
}

function Seed-VerifyCode {
	param(
		[string]$Email,
		[int]$Type,
		[string]$Code
	)
	$key = "user:verify_code:${Email}:$Type"
	docker exec lcchat-redis-1 redis-cli SETEX $key 300 $Code | Out-Null
}

function Collect-TraceLines {
	param(
		[string]$Since,
		[string]$TraceId,
		[string[]]$Services
	)
	if ($null -eq $Services -or $Services.Count -eq 0) {
		return @()
	}
	$raw = docker compose logs --no-color --since $Since @Services 2>&1 | Out-String -Width 4096
	return ($raw -split "`r?`n") | Where-Object { $_ -match [regex]::Escape($TraceId) }
}

function Add-Result {
	param([pscustomobject]$Result)
	$script:Results.Add($Result) | Out-Null
	return $Result
}

function Invoke-Endpoint {
	param(
		[string]$Name,
		[string]$Method,
		[string]$Path,
		[hashtable]$Headers = @{},
		[object]$Body = $null,
		[string[]]$Services = $CoreServices,
		[string[]]$FormParts = @(),
		[ValidateSet('json', 'text')][string]$Parse = 'json',
		[ValidateSet('success', 'text_success', 'health_success', 'business_error', 'expected_fail')][string]$Expectation = 'success',
		[string]$Note = ''
	)

	$traceId = New-TraceId
	$since = (Get-Date).ToUniversalTime().AddSeconds(-5).ToString('o')
	$http = 0
	$bodyText = ''
	$curlExit = 0

	if ($FormParts.Count -gt 0) {
		$args = @('-sS', '-m', '30', '-X', $Method, '-H', "X-Request-ID: $traceId")
		foreach ($key in $Headers.Keys) {
			$args += @('-H', "${key}: $($Headers[$key])")
		}
		foreach ($part in $FormParts) {
			$args += @('-F', $part)
		}
		$args += @("$BaseUrl$Path", '-w', "`nHTTPSTATUS:%{http_code}")
		$raw = (& curl.exe @args 2>&1 | Out-String)
		$curlExit = $LASTEXITCODE
		$match = [regex]::Match($raw, 'HTTPSTATUS:(\d+)\s*$')
		if ($match.Success) {
			$http = [int]$match.Groups[1].Value
			$bodyText = ([regex]::Replace($raw, 'HTTPSTATUS:\d+\s*$', '')).TrimEnd("`r", "`n")
		} else {
			$bodyText = $raw.TrimEnd("`r", "`n")
		}
	} else {
		$webHeaders = @{ 'X-Request-ID' = $traceId }
		foreach ($key in $Headers.Keys) {
			$webHeaders[$key] = $Headers[$key]
		}

		$webParams = @{
			UseBasicParsing = $true
			Headers         = $webHeaders
			Method          = $Method
			Uri             = "$BaseUrl$Path"
		}
		if ($null -ne $Body) {
			$webParams['ContentType'] = 'application/json'
			$webParams['Body'] = if ($Body -is [string]) { $Body } else { To-JsonText $Body }
		}

		try {
			$resp = Invoke-WebRequest @webParams
			$http = [int]$resp.StatusCode
			$bodyText = [string]$resp.Content
		} catch {
			if ($_.Exception.Response) {
				$response = $_.Exception.Response
				if ($_.ErrorDetails -and $_.ErrorDetails.Message) {
					$bodyText = $_.ErrorDetails.Message
				}
				if ($response -is [System.Net.Http.HttpResponseMessage]) {
					$http = [int]$response.StatusCode
					if ([string]::IsNullOrEmpty($bodyText) -and $response.Content) {
						try {
							$bodyText = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
						} catch {
						}
					}
					if ([string]::IsNullOrEmpty($bodyText)) {
						$bodyText = $_.Exception.Message
					}
				} else {
					$http = [int]$response.StatusCode.value__
					$reader = New-Object System.IO.StreamReader($response.GetResponseStream())
					$bodyText = $reader.ReadToEnd()
					$reader.Close()
				}
			} else {
				$curlExit = 1
				$bodyText = $_.Exception.Message
			}
		}
	}

	$traceLines = @()
	for ($attempt = 0; $attempt -lt 6; $attempt++) {
		Start-Sleep -Milliseconds 1000
		$traceLines = Collect-TraceLines -Since $since -TraceId $traceId -Services $Services
		if ($traceLines.Count -gt 0) {
			break
		}
	}
	$hasTraceLog = $traceLines.Count -gt 0

	$json = $null
	$code = $null
	$message = ''
	$data = $null
	if ($Parse -eq 'json' -and $bodyText) {
		try {
			$json = ($bodyText.Trim()) | ConvertFrom-Json -Depth 20 -ErrorAction Stop
			if ($null -ne $json.code) {
				$code = [int]$json.code
			}
			if ($null -ne $json.message) {
				$message = [string]$json.message
			}
			if ($null -ne $json.data) {
				$data = $json.data
			}
		} catch {
		}
		if ($null -eq $code -and $bodyText -match '"code"\s*:\s*(-?\d+)') {
			$code = [int]$Matches[1]
		}
		if ($message -eq '' -and $bodyText -match '"message"\s*:\s*"([^"]*)"') {
			$message = $Matches[1]
		}
	}

	$serviceHits = @()
	foreach ($line in $traceLines) {
		if ($line -match '^(?<svc>[^|]+)\|') {
			$serviceHits += $Matches['svc'].Trim()
		}
	}
	$serviceHits = $serviceHits | Sort-Object -Unique

	$status = 'FAIL'
	switch ($Expectation) {
		'success' {
			if ($curlExit -eq 0 -and $http -eq 200 -and $code -eq 0 -and $hasTraceLog) {
				$status = 'PASS'
			}
		}
		'text_success' {
			if ($curlExit -eq 0 -and $http -eq 200 -and $hasTraceLog) {
				$status = 'PASS'
			}
		}
		'health_success' {
			if ($curlExit -eq 0 -and $http -eq 200) {
				$status = if ($hasTraceLog) { 'PASS' } else { 'PASS_NO_HEALTH_LOG' }
			}
		}
		'business_error' {
			if ($curlExit -eq 0 -and $http -eq 200 -and $null -ne $code -and $code -ne 0 -and $code -lt 30000 -and $hasTraceLog) {
				$status = 'PASS_EXPECTED_BIZ_ERROR'
			}
		}
		'expected_fail' {
			if ($curlExit -eq 0 -and $http -ge 500 -and $hasTraceLog) {
				$status = 'EXPECTED_FAIL'
			}
		}
	}

	$result = [pscustomobject]@{
		name = $Name
		method = $Method
		path = $Path
		status = $status
		http = $http
		code = $code
		message = $message
		trace = $traceId
		note = $Note
		services = ($serviceHits -join ', ')
		log_count = $traceLines.Count
		log_preview = Short-Text (($traceLines | Select-Object -First 2) -join ' || ') 420
		response = Short-Text $bodyText 500
		data = $data
	}

	return Add-Result -Result $result
}

function Assert-Pass {
	param([pscustomobject]$Result)
	if ($Result.status -notmatch '^PASS') {
		throw "Step failed: $($Result.name) => $($Result.status); http=$($Result.http); code=$($Result.code); message=$($Result.message)"
	}
}

function Assert-ExpectedFail {
	param([pscustomobject]$Result)
	if ($Result.status -ne 'EXPECTED_FAIL') {
		throw "Step failed: $($Result.name) should be EXPECTED_FAIL, got $($Result.status); http=$($Result.http); code=$($Result.code)"
	}
}

function Assert-BizError {
	param([pscustomobject]$Result)
	if ($Result.status -ne 'PASS_EXPECTED_BIZ_ERROR') {
		throw "Step failed: $($Result.name) should be PASS_EXPECTED_BIZ_ERROR, got $($Result.status); http=$($Result.http); code=$($Result.code)"
	}
}

function Write-Summary {
	$script:Results | ForEach-Object {
		$line = @(
			'RESULT',
			$_.name,
			$_.status,
			$_.method,
			$_.path,
			"http=$($_.http)",
			"code=$($_.code)",
			"message=$($_.message)",
			"services=$($_.services)",
			"trace=$($_.trace)"
		) -join '|'
		Write-Output $line
	}

	$problematic = $script:Results | Where-Object {
		$_.status -notmatch '^PASS' -and $_.status -ne 'EXPECTED_FAIL' -and $_.status -ne 'PASS_EXPECTED_BIZ_ERROR'
	}
	if ($problematic.Count -gt 0) {
		Write-Output '---- DETAILS ----'
		foreach ($item in $problematic) {
			Write-Output ("DETAIL|{0}|response={1}" -f $item.name, $item.response)
			Write-Output ("DETAIL|{0}|logs={1}" -f $item.name, $item.log_preview)
		}
	}

	$expectedFail = $script:Results | Where-Object { $_.status -eq 'EXPECTED_FAIL' }
	if ($expectedFail.Count -gt 0) {
		Write-Output '---- EXPECTED FAILURES ----'
		foreach ($item in $expectedFail) {
			Write-Output ("EXPECTED_FAIL|{0}|response={1}" -f $item.name, $item.response)
			Write-Output ("EXPECTED_FAIL|{0}|logs={1}" -f $item.name, $item.log_preview)
		}
	}
}

try {
	$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
	$A = [ordered]@{ Email = "ta$stamp@example.com"; Password = 'PassA123'; Nick = 'TestA'; Tel = '13800000001'; Uuid = $null; Access = $null; Refresh = $null; Device1 = 'dev-a1'; Device2 = 'dev-a2'; NewEmail = "ta${stamp}n@example.com"; NewPassword = 'PassA456' }
	$B = [ordered]@{ Email = "tb$stamp@example.com"; Password = 'PassB123'; Nick = 'TestB'; Tel = '13800000002'; Uuid = $null; Access = $null; Refresh = $null; Device1 = 'dev-b1' }
	$C = [ordered]@{ Email = "tc$stamp@example.com"; Password = 'PassC123'; Nick = 'TestC'; Tel = '13800000003'; Uuid = $null; Access = $null; Refresh = $null; Device1 = 'dev-c1'; ResetPassword = 'PassC456' }

	Assert-Pass (Invoke-Endpoint -Name 'health' -Method 'GET' -Path '/health' -Services $GatewayOnly -Parse 'text' -Expectation 'health_success' -Note 'health route intentionally skips success logging')
	Assert-Pass (Invoke-Endpoint -Name 'metrics' -Method 'GET' -Path '/metrics' -Services $GatewayOnly -Parse 'text' -Expectation 'health_success' -Note 'metrics log was also manually verified beforehand')

	$sendVerify = Invoke-Endpoint -Name 'public.send-verify-code' -Method 'POST' -Path '/api/v1/public/user/send-verify-code' -Body @{ email = "send$stamp@example.com"; type = 1 } -Services @('gateway', 'auth') -Expectation 'expected_fail' -Note 'SMTP auth code is empty in current compose runtime'
	Assert-ExpectedFail $sendVerify

	Seed-VerifyCode -Email $A.Email -Type 1 -Code '111111'
	$verifyA = Invoke-Endpoint -Name 'public.verify-code' -Method 'POST' -Path '/api/v1/public/user/verify-code' -Body @{ email = $A.Email; verifyCode = '111111'; type = 1 } -Services @('gateway', 'auth')
	Assert-Pass $verifyA

	$registerA = Invoke-Endpoint -Name 'public.register.A' -Method 'POST' -Path '/api/v1/public/user/register' -Headers @{ 'X-Device-ID' = $A.Device1 } -Body @{ email = $A.Email; password = $A.Password; verifyCode = '111111'; nickname = $A.Nick; telephone = $A.Tel } -Services @('gateway', 'auth')
	Assert-Pass $registerA
	$A.Uuid = Get-PropValue $registerA.data @('userUuid', 'uuid')

	Seed-VerifyCode -Email $B.Email -Type 1 -Code '222222'
	$registerB = Invoke-Endpoint -Name 'public.register.B' -Method 'POST' -Path '/api/v1/public/user/register' -Headers @{ 'X-Device-ID' = $B.Device1 } -Body @{ email = $B.Email; password = $B.Password; verifyCode = '222222'; nickname = $B.Nick; telephone = $B.Tel } -Services @('gateway', 'auth')
	Assert-Pass $registerB
	$B.Uuid = Get-PropValue $registerB.data @('userUuid', 'uuid')

	Seed-VerifyCode -Email $C.Email -Type 1 -Code '333333'
	$registerC = Invoke-Endpoint -Name 'public.register.C' -Method 'POST' -Path '/api/v1/public/user/register' -Headers @{ 'X-Device-ID' = $C.Device1 } -Body @{ email = $C.Email; password = $C.Password; verifyCode = '333333'; nickname = $C.Nick; telephone = $C.Tel } -Services @('gateway', 'auth')
	Assert-Pass $registerC
	$C.Uuid = Get-PropValue $registerC.data @('userUuid', 'uuid')

	Start-Sleep -Seconds 8

	$loginA1 = Invoke-Endpoint -Name 'public.login.A.dev1' -Method 'POST' -Path '/api/v1/public/user/login' -Headers @{ 'X-Device-ID' = $A.Device1 } -Body @{ account = $A.Email; password = $A.Password; deviceInfo = @{ deviceName = 'A1'; platform = 'Windows'; osVersion = '10'; appVersion = '1.0.0' } } -Services @('gateway', 'auth')
	Assert-Pass $loginA1
	$A.Access = Get-PropValue $loginA1.data @('accessToken')
	$A.Refresh = Get-PropValue $loginA1.data @('refreshToken')
	$A.Uuid = Get-PropValue $loginA1.data.userInfo @('uuid', 'userUuid')

	$loginA2 = Invoke-Endpoint -Name 'public.login.A.dev2.setup' -Method 'POST' -Path '/api/v1/public/user/login' -Headers @{ 'X-Device-ID' = $A.Device2 } -Body @{ account = $A.Email; password = $A.Password; deviceInfo = @{ deviceName = 'A2'; platform = 'Web'; osVersion = 'browser'; appVersion = '1.0.0' } } -Services @('gateway', 'auth')
	Assert-Pass $loginA2

	$loginB1 = Invoke-Endpoint -Name 'public.login.B.dev1' -Method 'POST' -Path '/api/v1/public/user/login' -Headers @{ 'X-Device-ID' = $B.Device1 } -Body @{ account = $B.Email; password = $B.Password; deviceInfo = @{ deviceName = 'B1'; platform = 'Mac'; osVersion = '14'; appVersion = '1.0.0' } } -Services @('gateway', 'auth')
	Assert-Pass $loginB1
	$B.Access = Get-PropValue $loginB1.data @('accessToken')
	$B.Refresh = Get-PropValue $loginB1.data @('refreshToken')
	$B.Uuid = Get-PropValue $loginB1.data.userInfo @('uuid', 'userUuid')

	Seed-VerifyCode -Email $C.Email -Type 2 -Code '444444'
	$loginByCodeC = Invoke-Endpoint -Name 'public.login-by-code.C' -Method 'POST' -Path '/api/v1/public/user/login-by-code' -Headers @{ 'X-Device-ID' = $C.Device1 } -Body @{ email = $C.Email; verifyCode = '444444'; deviceInfo = @{ deviceName = 'C1'; platform = 'Android'; osVersion = '14'; appVersion = '1.0.0' } } -Services @('gateway', 'auth')
	Assert-Pass $loginByCodeC
	$C.Access = Get-PropValue $loginByCodeC.data @('accessToken')
	$C.Refresh = Get-PropValue $loginByCodeC.data @('refreshToken')
	$C.Uuid = Get-PropValue $loginByCodeC.data.userInfo @('uuid', 'userUuid')

	Seed-VerifyCode -Email $C.Email -Type 3 -Code '555555'
	Assert-Pass (Invoke-Endpoint -Name 'public.reset-password' -Method 'POST' -Path '/api/v1/public/user/reset-password' -Body @{ email = $C.Email; verifyCode = '555555'; newPassword = $C.ResetPassword } -Services @('gateway', 'auth'))

	$refreshA = Invoke-Endpoint -Name 'public.refresh-token' -Method 'POST' -Path '/api/v1/public/user/refresh-token' -Headers @{ 'X-Device-ID' = $A.Device1 } -Body @{ uuid = $A.Uuid; device_id = $A.Device1; refreshToken = $A.Refresh } -Services @('gateway', 'auth')
	Assert-Pass $refreshA

	Assert-Pass (Invoke-Endpoint -Name 'auth.user.profile.get' -Method 'GET' -Path '/api/v1/auth/user/profile' -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'user'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.profile.update' -Method 'PUT' -Path '/api/v1/auth/user/profile' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ nickname = 'TestA1'; gender = 1; birthday = '1995-06-15'; signature = 'sigA' } -Services @('gateway', 'user'))

	Seed-VerifyCode -Email $A.NewEmail -Type 4 -Code '666666'
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.change-email' -Method 'POST' -Path '/api/v1/auth/user/change-email' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ newEmail = $A.NewEmail; verifyCode = '666666' } -Services @('gateway', 'auth'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.batch-profile' -Method 'POST' -Path '/api/v1/auth/user/batch-profile' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ userUuids = @($A.Uuid, $B.Uuid, $C.Uuid) } -Services @('gateway', 'user'))

	$qrcodeA = Invoke-Endpoint -Name 'auth.user.qrcode' -Method 'GET' -Path '/api/v1/auth/user/qrcode' -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'user')
	Assert-Pass $qrcodeA
	$qrcodeToken = ((Get-PropValue $qrcodeA.data @('qrCode', 'qrcode')) -split '/')[-1]
	Assert-Pass (Invoke-Endpoint -Name 'public.parse-qrcode' -Method 'POST' -Path '/api/v1/public/user/parse-qrcode' -Body @{ token = $qrcodeToken } -Services @('gateway', 'user'))

	Assert-Pass (Invoke-Endpoint -Name 'auth.user.profile.other' -Method 'GET' -Path "/api/v1/auth/user/profile/$($B.Uuid)" -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'user', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.search' -Method 'GET' -Path '/api/v1/auth/user/search?keyword=TestB&page=1&pageSize=20' -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'user', 'relation'))

	$avatarPath = Join-Path $env:TEMP ("lcchat-avatar-" + [guid]::NewGuid().ToString() + '.png')
	[System.IO.File]::WriteAllText($avatarPath, 'fakepng')
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.avatar' -Method 'POST' -Path '/api/v1/auth/user/avatar' -Headers @{ Authorization = "Bearer $($A.Access)" } -FormParts @("avatar=@$avatarPath;type=image/png;filename=test.png") -Services @('gateway', 'user'))

	Assert-Pass (Invoke-Endpoint -Name 'auth.user.devices.list.before-kick' -Method 'GET' -Path '/api/v1/auth/user/devices' -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'auth'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.online-status' -Method 'GET' -Path "/api/v1/auth/user/online-status/$($B.Uuid)" -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'auth'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.batch-online-status' -Method 'POST' -Path '/api/v1/auth/user/batch-online-status' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ userUuids = @($A.Uuid, $B.Uuid, $C.Uuid) } -Services @('gateway', 'auth'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.devices.kick' -Method 'DELETE' -Path "/api/v1/auth/user/devices/$($A.Device2)" -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'auth'))

	$applyA = Invoke-Endpoint -Name 'auth.friend.apply' -Method 'POST' -Path '/api/v1/auth/friend/apply' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ targetUuid = $B.Uuid; reason = 'hello'; source = 'search' } -Services @('gateway', 'relation')
	Assert-Pass $applyA
	$applyId = [int64](Get-PropValue $applyA.data @('applyId'))

	Assert-Pass (Invoke-Endpoint -Name 'auth.friend.apply.unread' -Method 'GET' -Path '/api/v1/auth/friend/apply/unread' -Headers @{ Authorization = "Bearer $($B.Access)" } -Services @('gateway', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.friend.apply.sent' -Method 'GET' -Path '/api/v1/auth/friend/apply/sent?status=-1&page=1&pageSize=20' -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.friend.apply.read' -Method 'POST' -Path '/api/v1/auth/friend/apply/read' -Headers @{ Authorization = "Bearer $($B.Access)" } -Body @{ applyIds = @($applyId) } -Services @('gateway', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.friend.apply-list' -Method 'GET' -Path '/api/v1/auth/friend/apply-list?status=-1&page=1&pageSize=20' -Headers @{ Authorization = "Bearer $($B.Access)" } -Services @('gateway', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.friend.apply.handle' -Method 'POST' -Path '/api/v1/auth/friend/apply/handle' -Headers @{ Authorization = "Bearer $($B.Access)" } -Body @{ applyId = $applyId; action = 1; remark = 'accepted' } -Services @('gateway', 'relation'))

	Assert-Pass (Invoke-Endpoint -Name 'auth.friend.check' -Method 'POST' -Path '/api/v1/auth/friend/check' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ userUuid = $A.Uuid; peerUuid = $B.Uuid } -Services @('gateway', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.friend.relation' -Method 'POST' -Path '/api/v1/auth/friend/relation' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ userUuid = $A.Uuid; peerUuid = $B.Uuid } -Services @('gateway', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.friend.remark' -Method 'POST' -Path '/api/v1/auth/friend/remark' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ userUuid = $B.Uuid; remark = 'buddy' } -Services @('gateway', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.friend.tag' -Method 'POST' -Path '/api/v1/auth/friend/tag' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ userUuid = $B.Uuid; groupTag = 'team' } -Services @('gateway', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.friend.list' -Method 'GET' -Path '/api/v1/auth/friend/list?page=1&pageSize=20' -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.friend.sync' -Method 'POST' -Path '/api/v1/auth/friend/sync' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ version = 0; limit = 100 } -Services @('gateway', 'relation'))
	$friendTags = Invoke-Endpoint -Name 'auth.friend.tags' -Method 'GET' -Path '/api/v1/auth/friend/tags' -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'relation') -Expectation 'business_error'
	Assert-BizError $friendTags

	$clientMsgId = 'cli-' + ([guid]::NewGuid().ToString())
	$sendMsg = Invoke-Endpoint -Name 'auth.messages.send' -Method 'POST' -Path '/api/v1/auth/messages/send' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ clientMsgId = $clientMsgId; convType = 1; targetUuid = $B.Uuid; msgType = 1; content = '{"text":"hello from A"}' } -Services @('gateway', 'msg', 'relation', 'user')
	Assert-Pass $sendMsg
	$convId = [string](Get-PropValue $sendMsg.data @('convId'))
	$msgId = [string](Get-PropValue $sendMsg.data @('msgId'))
	$seq = [int64](Get-PropValue $sendMsg.data @('seq'))
	$escapedConvId = [uri]::EscapeDataString($convId)

	Start-Sleep -Seconds 2
	Assert-Pass (Invoke-Endpoint -Name 'auth.conversations.list' -Method 'GET' -Path '/api/v1/auth/conversations?updatedSince=0&pageSize=50' -Headers @{ Authorization = "Bearer $($B.Access)" } -Services @('gateway', 'msg'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.messages.pull' -Method 'GET' -Path "/api/v1/auth/messages/pull?convId=$escapedConvId&anchorSeq=0&limit=20&direction=2" -Headers @{ Authorization = "Bearer $($B.Access)" } -Services @('gateway', 'msg'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.messages.get-by-ids' -Method 'POST' -Path '/api/v1/auth/messages/get-by-ids' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ convId = $convId; msgIds = @($msgId) } -Services @('gateway', 'msg'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.conversations.mark-read' -Method 'POST' -Path '/api/v1/auth/conversations/mark-read' -Headers @{ Authorization = "Bearer $($B.Access)" } -Body @{ convId = $convId; readSeq = $seq } -Services @('gateway', 'msg'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.conversations.settings' -Method 'PATCH' -Path '/api/v1/auth/conversations/settings' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ convId = $convId; mute = $true; pin = $true } -Services @('gateway', 'msg'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.messages.recall' -Method 'POST' -Path '/api/v1/auth/messages/recall' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ convId = $convId; msgId = $msgId } -Services @('gateway', 'msg'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.conversations.delete' -Method 'DELETE' -Path "/api/v1/auth/conversations/$escapedConvId" -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'msg'))

	Assert-Pass (Invoke-Endpoint -Name 'auth.friend.delete' -Method 'POST' -Path '/api/v1/auth/friend/delete' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ userUuid = $B.Uuid } -Services @('gateway', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.blacklist.add' -Method 'POST' -Path '/api/v1/auth/blacklist' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ targetUuid = $B.Uuid } -Services @('gateway', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.blacklist.list' -Method 'GET' -Path '/api/v1/auth/blacklist?page=1&pageSize=20' -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.blacklist.check' -Method 'POST' -Path '/api/v1/auth/blacklist/check' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ userUuid = $A.Uuid; targetUuid = $B.Uuid } -Services @('gateway', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.blacklist.remove' -Method 'DELETE' -Path "/api/v1/auth/blacklist/$($B.Uuid)" -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'relation'))

	Assert-Pass (Invoke-Endpoint -Name 'auth.user.change-password' -Method 'POST' -Path '/api/v1/auth/user/change-password' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ oldPassword = $A.Password; newPassword = $A.NewPassword } -Services @('gateway', 'auth'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.logout' -Method 'POST' -Path '/api/v1/auth/user/logout' -Headers @{ Authorization = "Bearer $($B.Access)" } -Body @{ deviceId = $B.Device1 } -Services @('gateway', 'auth'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.delete-account' -Method 'POST' -Path '/api/v1/auth/user/delete-account' -Headers @{ Authorization = "Bearer $($C.Access)" } -Body @{ password = $C.ResetPassword; reason = 'test cleanup' } -Services @('gateway', 'auth'))

	Write-Summary
} catch {
	Write-Summary
	throw
}
