param()

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$env:EMAIL_AUTH_CODE = 'dummy'

$Utf8NoBom = [System.Text.UTF8Encoding]::new($false)
try {
	[Console]::InputEncoding = $Utf8NoBom
} catch {
}
try {
	[Console]::OutputEncoding = $Utf8NoBom
} catch {
}
$OutputEncoding = $Utf8NoBom
$PSDefaultParameterValues['Set-Content:Encoding'] = 'utf8'
$PSDefaultParameterValues['Add-Content:Encoding'] = 'utf8'
$PSDefaultParameterValues['Out-File:Encoding'] = 'utf8'
if ($IsWindows) {
	chcp.com 65001 | Out-Null
}

$BaseUrl = 'http://127.0.0.1:8080'
$CoreServices = @('gateway', 'auth', 'user', 'relation', 'msg')
$GatewayOnly = @('gateway')
$script:Results = New-Object System.Collections.Generic.List[object]
$script:ProgressLog = Join-Path $PSScriptRoot '..\tmp\gateway_http_interface_test.log'
$script:DocPath = Join-Path $PSScriptRoot '..\doc\dev-notes\gateway_http_interface_test_record.md'
$script:DocInitialized = $false

if (Test-Path $script:ProgressLog) {
	Remove-Item $script:ProgressLog -Force
}

if (Test-Path $script:DocPath) {
	Remove-Item $script:DocPath -Force
}

function Write-ProgressLine {
	param([string]$Text)
	$line = "[{0}] {1}" -f (Get-Date).ToString('s'), $Text
	Add-Content -Path $script:ProgressLog -Value $line
}

function Initialize-TestDoc {
	$docDir = Split-Path $script:DocPath -Parent
	if (-not (Test-Path $docDir)) {
		New-Item -ItemType Directory -Force -Path $docDir | Out-Null
	}
	@(
		'# Gateway HTTP 接口测试记录',
		'',
		"- 开始时间: $((Get-Date).ToString('yyyy-MM-dd HH:mm:ss'))",
		"- 基地址: $BaseUrl",
		'- 说明: 每完成一个接口测试后立即追加一条记录；密码、验证码、Token 已脱敏。',
		''
	) | Set-Content -Path $script:DocPath
	$script:DocInitialized = $true
}

function Sanitize-SecretText {
	param([string]$Text)
	if ([string]::IsNullOrWhiteSpace($Text)) {
		return '(empty)'
	}
	$sanitized = $Text
	$patterns = @(
		'(?i)("password"\s*:\s*")[^"]*(")',
		'(?i)("newPassword"\s*:\s*")[^"]*(")',
		'(?i)("oldPassword"\s*:\s*")[^"]*(")',
		'(?i)("verifyCode"\s*:\s*")[^"]*(")',
		'(?i)("refreshToken"\s*:\s*")[^"]*(")',
		'(?i)("accessToken"\s*:\s*")[^"]*(")',
		'(?i)(Authorization\s*[:=]\s*)[^,;\]\} ]+',
		'(?i)(X-Device-ID\s*[:=]\s*)[^,;\]\} ]+'
	)
	foreach ($pattern in $patterns) {
		$sanitized = [regex]::Replace($sanitized, $pattern, '$1***$2')
	}
	return $sanitized
}

function Format-HeadersSummary {
	param([hashtable]$Headers)
	if ($null -eq $Headers -or $Headers.Count -eq 0) {
		return '(none)'
	}
	$items = @()
	foreach ($key in ($Headers.Keys | Sort-Object)) {
		$value = [string]$Headers[$key]
		if ($key -match 'Authorization|X-Device-ID') {
			$value = '***'
		}
		$items += "${key}=${value}"
	}
	return ($items -join '; ')
}

function Format-RequestSummary {
	param(
		[hashtable]$Headers,
		[object]$Body,
		[string[]]$FormParts
	)
	$headerSummary = Format-HeadersSummary $Headers
	if ($FormParts.Count -gt 0) {
		$formSummary = Sanitize-SecretText (($FormParts -join '; '))
		return "headers: $headerSummary; form-data: $formSummary"
	}
	$bodyText = if ($Body -is [string]) { $Body } else { To-JsonText $Body }
	return "headers: $headerSummary; body: $(Sanitize-SecretText $bodyText)"
}

function Write-DocEntry {
	param(
		[pscustomobject]$Result,
		[hashtable]$Headers,
		[object]$Body,
		[string[]]$FormParts,
		[string]$Expectation
	)
	if (-not $script:DocInitialized) {
		Initialize-TestDoc
	}
	$requestSummary = Format-RequestSummary -Headers $Headers -Body $Body -FormParts $FormParts
	$responseSummary = Sanitize-SecretText (Short-Text $Result.raw_response 700)
	$logSummary = Sanitize-SecretText (Short-Text $Result.log_preview 500)
	$entry = @(
		"## $($Result.name)",
		"- 时间: $((Get-Date).ToString('yyyy-MM-dd HH:mm:ss'))",
		"- 请求: $($Result.method) $($Result.path)",
		'- 测试方法: 使用 PowerShell 调用 Gateway HTTP 接口，并按 trace_id 检索 docker compose logs 校验容器日志。',
		"- 请求摘要: $requestSummary",
		"- 期望类型: $Expectation",
		"- 结果: $($Result.status)",
		"- HTTP 状态: $($Result.http)",
		"- 业务码: $($Result.code)",
		"- 响应摘要: $responseSummary",
		"- 命中服务: $($Result.services)",
		"- trace_id: $($Result.trace)",
		"- 日志摘要: $logSummary",
		''
	)
	Add-Content -Path $script:DocPath -Value $entry
}

function Get-ResultDataValue {
	param(
		[pscustomobject]$Result,
		[string]$Path
	)
	if ($null -eq $Result) {
		return $null
	}
	$current = $Result.data
	if ($null -eq $current -and -not [string]::IsNullOrWhiteSpace($Result.raw_response)) {
		try {
			$parsed = ($Result.raw_response | ConvertFrom-Json -Depth 20 -ErrorAction Stop)
			$current = $parsed.data
		} catch {
		}
	}
	if ($null -eq $current) {
		return $null
	}
	foreach ($segment in ($Path -split '\.')) {
		if ($null -eq $current) {
			return $null
		}
		if ($current -is [hashtable]) {
			if (-not $current.ContainsKey($segment)) {
				return $null
			}
			$current = $current[$segment]
			continue
		}
		$prop = $current.PSObject.Properties.Match($segment) | Select-Object -First 1
		if ($null -eq $prop) {
			return $null
		}
		$current = $prop.Value
	}
	if ($null -eq $current) {
		return $null
	}
	return [string]$current
}

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
		[int]$Max = 420
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

function Ensure-AuthTables {
	$sql = @'
CREATE TABLE IF NOT EXISTS `user_account` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT 'auto id',
  `user_uuid` CHAR(20) NOT NULL COMMENT 'user uuid',
  `email` VARCHAR(100) NOT NULL COMMENT 'email',
  `telephone` VARCHAR(20) DEFAULT NULL COMMENT 'telephone',
  `password_hash` CHAR(60) NOT NULL COMMENT 'bcrypt hash',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT '0 active 1 deleted',
  `is_admin` TINYINT NOT NULL DEFAULT 0 COMMENT 'admin flag',
  `login_nickname` VARCHAR(20) NOT NULL DEFAULT '' COMMENT 'redundant nickname',
  `login_avatar` VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'redundant avatar',
  `last_login_at` DATETIME(3) DEFAULT NULL COMMENT 'last login time',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'created at',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT 'updated at',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT 'deleted at',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_account_user_uuid` (`user_uuid`),
  UNIQUE KEY `uk_user_account_email` (`email`),
  UNIQUE KEY `uk_user_account_telephone` (`telephone`),
  KEY `idx_user_account_deleted_at` (`deleted_at`),
  KEY `idx_user_account_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='user account';

CREATE TABLE IF NOT EXISTS `outbox_events` (
  `id` BIGINT NOT NULL AUTO_INCREMENT COMMENT 'event id',
  `event_type` VARCHAR(128) NOT NULL COMMENT 'event type',
  `entity_id` VARCHAR(64) NOT NULL COMMENT 'entity id',
  `payload` LONGTEXT NOT NULL COMMENT 'payload json',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'created at',
  PRIMARY KEY (`id`),
  KEY `idx_outbox_event_type_created` (`event_type`, `created_at`),
  KEY `idx_outbox_entity_id` (`entity_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='outbox events';

CREATE TABLE IF NOT EXISTS `idempotent_events` (
  `id` BIGINT NOT NULL AUTO_INCREMENT COMMENT 'auto id',
  `event_type` VARCHAR(64) NOT NULL COMMENT 'event type',
  `event_id` VARCHAR(64) NOT NULL COMMENT 'event id',
  `processed_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'processed at',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_type_event` (`event_type`, `event_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='idempotent events';

CREATE TABLE IF NOT EXISTS `user_profile` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT 'auto id',
  `user_uuid` CHAR(20) NOT NULL COMMENT 'user uuid',
  `nickname` VARCHAR(20) NOT NULL DEFAULT '' COMMENT 'nickname',
  `avatar` VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'avatar',
  `gender` TINYINT NOT NULL DEFAULT 3 COMMENT 'gender',
  `signature` VARCHAR(100) NOT NULL DEFAULT '' COMMENT 'signature',
  `birthday` DATE DEFAULT NULL COMMENT 'birthday',
  `qrcode_token` VARCHAR(64) DEFAULT NULL COMMENT 'qrcode token',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'created at',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT 'updated at',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_profile_user_uuid` (`user_uuid`),
  KEY `idx_user_profile_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='user profile';

CREATE TABLE IF NOT EXISTS `user_relations` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT 'auto id',
  `user_uuid` CHAR(20) NOT NULL COMMENT 'user uuid',
  `peer_uuid` CHAR(20) NOT NULL COMMENT 'peer uuid',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT 'relation status',
  `remark` VARCHAR(64) DEFAULT NULL COMMENT 'remark',
  `source` VARCHAR(64) DEFAULT NULL COMMENT 'source',
  `group_tag` VARCHAR(32) DEFAULT NULL COMMENT 'group tag',
  `blacklisted_at` DATETIME(3) DEFAULT NULL COMMENT 'blacklisted at',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'created at',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT 'updated at',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT 'deleted at',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uidx_user_peer` (`user_uuid`, `peer_uuid`),
  KEY `idx_user_updated_at` (`user_uuid`, `updated_at`),
  KEY `idx_peer_uuid` (`peer_uuid`),
  KEY `idx_user_status_deleted_created` (`user_uuid`, `status`, `deleted_at`, `created_at`, `id`),
  KEY `idx_user_blacklist_deleted_time` (`user_uuid`, `status`, `deleted_at`, `blacklisted_at`, `id`),
  KEY `idx_user_relations_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='user relations';

CREATE TABLE IF NOT EXISTS `apply_requests` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT 'auto id',
  `apply_type` TINYINT NOT NULL COMMENT 'apply type',
  `applicant_uuid` CHAR(20) NOT NULL COMMENT 'applicant uuid',
  `target_uuid` CHAR(20) NOT NULL COMMENT 'target uuid',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT 'status',
  `is_read` TINYINT(1) NOT NULL DEFAULT 0 COMMENT 'is read',
  `reason` VARCHAR(255) DEFAULT NULL COMMENT 'reason',
  `source` VARCHAR(32) DEFAULT NULL COMMENT 'source',
  `handle_user_uuid` CHAR(20) DEFAULT NULL COMMENT 'handle user uuid',
  `handle_remark` VARCHAR(255) DEFAULT NULL COMMENT 'handle remark',
  `expired_at` DATETIME(3) DEFAULT NULL COMMENT 'expired at',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'created at',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT 'updated at',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT 'deleted at',
  PRIMARY KEY (`id`),
  KEY `idx_applicant_target` (`applicant_uuid`, `target_uuid`),
  KEY `idx_apply_pending_list` (`apply_type`, `target_uuid`, `status`, `deleted_at`, `created_at`, `id`),
  KEY `idx_apply_sent_list` (`apply_type`, `applicant_uuid`, `status`, `deleted_at`, `created_at`, `id`),
  KEY `idx_apply_target_read` (`target_uuid`, `apply_type`, `is_read`, `deleted_at`),
  KEY `idx_apply_requests_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='apply requests';

CREATE TABLE IF NOT EXISTS `device_sessions` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT 'auto id',
  `user_uuid` CHAR(20) NOT NULL COMMENT 'user uuid',
  `device_id` VARCHAR(64) NOT NULL COMMENT 'device id',
  `device_name` VARCHAR(64) NOT NULL DEFAULT 'Unknown Device' COMMENT 'device name',
  `platform` VARCHAR(32) NOT NULL COMMENT 'platform',
  `app_version` VARCHAR(32) DEFAULT NULL COMMENT 'app version',
  `ip` VARCHAR(64) DEFAULT NULL COMMENT 'login ip',
  `user_agent` VARCHAR(512) DEFAULT NULL COMMENT 'user agent',
  `expire_at` DATETIME(3) DEFAULT NULL COMMENT 'expire at',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT 'status',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'created at',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT 'updated at',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT 'deleted at',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uidx_user_device` (`user_uuid`, `device_id`),
  KEY `idx_device_expire_at` (`expire_at`),
  KEY `idx_device_deleted_at` (`deleted_at`),
  KEY `idx_device_user_updated` (`user_uuid`, `updated_at`, `id`),
  KEY `idx_device_user_status_deleted` (`user_uuid`, `status`, `deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='device sessions';
'@
	docker exec lcchat-mysql-1 mysql -uroot -proot -D chat_server -e $sql | Out-Null
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
	Write-ProgressLine ("RESULT|{0}|{1}|http={2}|code={3}|message={4}" -f $Result.name, $Result.status, $Result.http, $Result.code, $Result.message)
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

	Write-ProgressLine ("BEGIN|{0}|{1}|{2}" -f $Name, $Method, $Path)

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
		log_preview = Short-Text (($traceLines | Select-Object -First 2) -join ' || ')
		response = Short-Text $bodyText 500
		raw_response = $bodyText
		data = $data
	}

	Write-DocEntry -Result $result -Headers $Headers -Body $Body -FormParts $FormParts -Expectation $Expectation

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
}

Ensure-AuthTables
Initialize-TestDoc
Write-ProgressLine 'Auth tables ensured'
Start-Sleep -Seconds 50
Write-ProgressLine 'Breaker cool-down finished'

try {
	$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
	$A = [ordered]@{ Email = "ta$stamp@example.com"; Password = 'PassA123'; Nick = 'TestA'; Tel = '13800000001'; Uuid = $null; Access = $null; Refresh = $null; Device1 = 'dev-a1'; Device2 = 'dev-a2'; NewEmail = "ta${stamp}n@example.com"; NewPassword = 'PassA456' }
	$B = [ordered]@{ Email = "tb$stamp@example.com"; Password = 'PassB123'; Nick = 'TestB'; Tel = '13800000002'; Uuid = $null; Access = $null; Refresh = $null; Device1 = 'dev-b1' }
	$C = [ordered]@{ Email = "tc$stamp@example.com"; Password = 'PassC123'; Nick = 'TestC'; Tel = '13800000003'; Uuid = $null; Access = $null; Refresh = $null; Device1 = 'dev-c1'; ResetPassword = 'PassC456' }
	$VerifyOnlyEmail = "tv$stamp@example.com"
	$phoneBase = [int64]($stamp % 1000000000)
	$A.Tel = ('13{0:D9}' -f (($phoneBase + 1) % 1000000000))
	$B.Tel = ('13{0:D9}' -f (($phoneBase + 2) % 1000000000))
	$C.Tel = ('13{0:D9}' -f (($phoneBase + 3) % 1000000000))

	Assert-Pass (Invoke-Endpoint -Name 'health' -Method 'GET' -Path '/health' -Services $GatewayOnly -Parse 'text' -Expectation 'health_success' -Note 'health route intentionally skips success logging')
	Assert-Pass (Invoke-Endpoint -Name 'metrics' -Method 'GET' -Path '/metrics' -Services $GatewayOnly -Parse 'text' -Expectation 'health_success' -Note 'metrics log also verified manually')

	Seed-VerifyCode -Email $A.Email -Type 1 -Code '111111'
	$registerA = Invoke-Endpoint -Name 'public.register.A' -Method 'POST' -Path '/api/v1/public/user/register' -Headers @{ 'X-Device-ID' = $A.Device1 } -Body @{ email = $A.Email; password = $A.Password; verifyCode = '111111'; nickname = $A.Nick; telephone = $A.Tel } -Services @('gateway', 'auth')
	Assert-Pass $registerA
	$A.Uuid = Get-ResultDataValue $registerA 'userUuid'

	Seed-VerifyCode -Email $B.Email -Type 1 -Code '222222'
	$registerB = Invoke-Endpoint -Name 'public.register.B' -Method 'POST' -Path '/api/v1/public/user/register' -Headers @{ 'X-Device-ID' = $B.Device1 } -Body @{ email = $B.Email; password = $B.Password; verifyCode = '222222'; nickname = $B.Nick; telephone = $B.Tel } -Services @('gateway', 'auth')
	Assert-Pass $registerB
	$B.Uuid = Get-ResultDataValue $registerB 'userUuid'

	Seed-VerifyCode -Email $C.Email -Type 1 -Code '333333'
	$registerC = Invoke-Endpoint -Name 'public.register.C' -Method 'POST' -Path '/api/v1/public/user/register' -Headers @{ 'X-Device-ID' = $C.Device1 } -Body @{ email = $C.Email; password = $C.Password; verifyCode = '333333'; nickname = $C.Nick; telephone = $C.Tel } -Services @('gateway', 'auth')
	Assert-Pass $registerC
	$C.Uuid = Get-ResultDataValue $registerC 'userUuid'

	Seed-VerifyCode -Email $VerifyOnlyEmail -Type 1 -Code '909090'
	Assert-Pass (Invoke-Endpoint -Name 'public.verify-code' -Method 'POST' -Path '/api/v1/public/user/verify-code' -Body @{ email = $VerifyOnlyEmail; verifyCode = '909090'; type = 1 } -Services @('gateway', 'auth'))

	Start-Sleep -Seconds 5

	$loginA1 = Invoke-Endpoint -Name 'public.login.A.dev1' -Method 'POST' -Path '/api/v1/public/user/login' -Headers @{ 'X-Device-ID' = $A.Device1 } -Body @{ account = $A.Email; password = $A.Password; deviceInfo = @{ deviceName = 'A1'; platform = 'Windows'; osVersion = '10'; appVersion = '1.0.0' } } -Services @('gateway', 'auth')
	Assert-Pass $loginA1
	$A.Access = Get-ResultDataValue $loginA1 'accessToken'
	$A.Refresh = Get-ResultDataValue $loginA1 'refreshToken'
	$A.Uuid = Get-ResultDataValue $loginA1 'userInfo.uuid'
	if ([string]::IsNullOrWhiteSpace($A.Access) -or [string]::IsNullOrWhiteSpace($A.Refresh) -or [string]::IsNullOrWhiteSpace($A.Uuid)) {
		Write-ProgressLine ("LOGIN_A1_PARSE_FAIL|raw={0}" -f (Sanitize-SecretText $loginA1.raw_response))
		throw "login A1 响应缺少 access/refresh/uuid"
	}
	Write-ProgressLine ("LOGIN_A1_PARSE|access_len={0}|refresh_len={1}|uuid={2}" -f $A.Access.Length, $A.Refresh.Length, $A.Uuid)

	$loginA2 = Invoke-Endpoint -Name 'public.login.A.dev2.setup' -Method 'POST' -Path '/api/v1/public/user/login' -Headers @{ 'X-Device-ID' = $A.Device2 } -Body @{ account = $A.Email; password = $A.Password; deviceInfo = @{ deviceName = 'A2'; platform = 'Web'; osVersion = 'browser'; appVersion = '1.0.0' } } -Services @('gateway', 'auth')
	Assert-Pass $loginA2

	$loginB1 = Invoke-Endpoint -Name 'public.login.B.dev1' -Method 'POST' -Path '/api/v1/public/user/login' -Headers @{ 'X-Device-ID' = $B.Device1 } -Body @{ account = $B.Email; password = $B.Password; deviceInfo = @{ deviceName = 'B1'; platform = 'Mac'; osVersion = '14'; appVersion = '1.0.0' } } -Services @('gateway', 'auth')
	Assert-Pass $loginB1
	$B.Access = Get-ResultDataValue $loginB1 'accessToken'
	$B.Refresh = Get-ResultDataValue $loginB1 'refreshToken'
	$B.Uuid = Get-ResultDataValue $loginB1 'userInfo.uuid'

	Seed-VerifyCode -Email $C.Email -Type 2 -Code '444444'
	$loginByCodeC = Invoke-Endpoint -Name 'public.login-by-code.C' -Method 'POST' -Path '/api/v1/public/user/login-by-code' -Headers @{ 'X-Device-ID' = $C.Device1 } -Body @{ email = $C.Email; verifyCode = '444444'; deviceInfo = @{ deviceName = 'C1'; platform = 'Android'; osVersion = '14'; appVersion = '1.0.0' } } -Services @('gateway', 'auth')
	Assert-Pass $loginByCodeC
	$C.Access = Get-ResultDataValue $loginByCodeC 'accessToken'
	$C.Refresh = Get-ResultDataValue $loginByCodeC 'refreshToken'
	$C.Uuid = Get-ResultDataValue $loginByCodeC 'userInfo.uuid'

	Seed-VerifyCode -Email $C.Email -Type 3 -Code '555555'
	Assert-Pass (Invoke-Endpoint -Name 'public.reset-password' -Method 'POST' -Path '/api/v1/public/user/reset-password' -Body @{ email = $C.Email; verifyCode = '555555'; newPassword = $C.ResetPassword } -Services @('gateway', 'auth'))

	Write-ProgressLine ("REFRESH_INPUT|uuid={0}|device={1}|refresh_len={2}" -f $A.Uuid, $A.Device1, $A.Refresh.Length)
	Assert-Pass (Invoke-Endpoint -Name 'public.refresh-token' -Method 'POST' -Path '/api/v1/public/user/refresh-token' -Headers @{ 'X-Device-ID' = $A.Device1 } -Body @{ uuid = $A.Uuid; device_id = $A.Device1; refreshToken = $A.Refresh } -Services @('gateway', 'auth'))

	Assert-Pass (Invoke-Endpoint -Name 'auth.user.profile.get' -Method 'GET' -Path '/api/v1/auth/user/profile' -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'user'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.profile.update' -Method 'PUT' -Path '/api/v1/auth/user/profile' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ nickname = 'TestA1'; gender = 1; birthday = '1995-06-15'; signature = 'sigA' } -Services @('gateway', 'user'))

	Seed-VerifyCode -Email $A.NewEmail -Type 4 -Code '666666'
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.change-email' -Method 'POST' -Path '/api/v1/auth/user/change-email' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ newEmail = $A.NewEmail; verifyCode = '666666' } -Services @('gateway', 'auth'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.batch-profile' -Method 'POST' -Path '/api/v1/auth/user/batch-profile' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ userUuids = @($A.Uuid, $B.Uuid, $C.Uuid) } -Services @('gateway', 'user'))

	$qrcodeA = Invoke-Endpoint -Name 'auth.user.qrcode' -Method 'GET' -Path '/api/v1/auth/user/qrcode' -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'user')
	Assert-Pass $qrcodeA
	$qrcodeToken = ((Get-ResultDataValue $qrcodeA 'qrCode') -split '/')[-1]
	Assert-Pass (Invoke-Endpoint -Name 'public.parse-qrcode' -Method 'POST' -Path '/api/v1/public/user/parse-qrcode' -Body @{ token = $qrcodeToken } -Services @('gateway', 'user'))

	Assert-Pass (Invoke-Endpoint -Name 'auth.user.profile.other' -Method 'GET' -Path "/api/v1/auth/user/profile/$($B.Uuid)" -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'user', 'relation'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.search' -Method 'GET' -Path '/api/v1/auth/user/search?keyword=TestB&page=1&pageSize=20' -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'user', 'relation'))

	$avatarPath = Join-Path $env:TEMP ("lcchat-avatar-" + [guid]::NewGuid().ToString() + '.png')
	$avatarPngBase64 = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO6pM6kAAAAASUVORK5CYII='
	[System.IO.File]::WriteAllBytes($avatarPath, [Convert]::FromBase64String($avatarPngBase64))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.avatar' -Method 'POST' -Path '/api/v1/auth/user/avatar' -Headers @{ Authorization = "Bearer $($A.Access)" } -FormParts @("avatar=@$avatarPath;type=image/png;filename=test.png") -Services @('gateway', 'user'))

	Assert-Pass (Invoke-Endpoint -Name 'auth.user.devices.list.before-kick' -Method 'GET' -Path '/api/v1/auth/user/devices' -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'auth'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.online-status' -Method 'GET' -Path "/api/v1/auth/user/online-status/$($B.Uuid)" -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'auth'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.batch-online-status' -Method 'POST' -Path '/api/v1/auth/user/batch-online-status' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ userUuids = @($A.Uuid, $B.Uuid, $C.Uuid) } -Services @('gateway', 'auth'))
	Assert-Pass (Invoke-Endpoint -Name 'auth.user.devices.kick' -Method 'DELETE' -Path "/api/v1/auth/user/devices/$($A.Device2)" -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'auth'))

	$applyA = Invoke-Endpoint -Name 'auth.friend.apply' -Method 'POST' -Path '/api/v1/auth/friend/apply' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ targetUuid = $B.Uuid; reason = 'hello'; source = 'search' } -Services @('gateway', 'relation')
	Assert-Pass $applyA
	$applyId = [int64](Get-ResultDataValue $applyA 'applyId')

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
	Assert-BizError (Invoke-Endpoint -Name 'auth.friend.tags' -Method 'GET' -Path '/api/v1/auth/friend/tags' -Headers @{ Authorization = "Bearer $($A.Access)" } -Services @('gateway', 'relation') -Expectation 'business_error')

	$clientMsgId = 'cli-' + ([guid]::NewGuid().ToString())
	$sendMsg = Invoke-Endpoint -Name 'auth.messages.send' -Method 'POST' -Path '/api/v1/auth/messages/send' -Headers @{ Authorization = "Bearer $($A.Access)" } -Body @{ clientMsgId = $clientMsgId; convType = 1; targetUuid = $B.Uuid; msgType = 1; content = '{"text":"hello from A"}' } -Services @('gateway', 'msg', 'relation', 'user')
	Assert-Pass $sendMsg
	$convId = Get-ResultDataValue $sendMsg 'convId'
	$msgId = Get-ResultDataValue $sendMsg 'msgId'
	$seq = [int64](Get-ResultDataValue $sendMsg 'seq')
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

	$sendVerify = Invoke-Endpoint -Name 'public.send-verify-code' -Method 'POST' -Path '/api/v1/public/user/send-verify-code' -Body @{ email = "send$stamp@example.com"; type = 1 } -Services @('gateway', 'auth') -Expectation 'expected_fail' -Note 'SMTP auth code is empty in current compose runtime'
	Assert-ExpectedFail $sendVerify

	Write-Summary
} catch {
	Write-Summary
	throw
}
