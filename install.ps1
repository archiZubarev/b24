# Установка b24 и скилла bitrix24 для Claude Code на Windows.
#   irm https://raw.githubusercontent.com/archiZubarev/b24/main/install.ps1 | iex
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$repo = 'archiZubarev/b24'
$base = "https://github.com/$repo/releases/latest/download"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$exeName = "b24-windows-$arch.exe"
$binDir = Join-Path $env:LOCALAPPDATA 'Programs\b24'
$skills = Join-Path $env:USERPROFILE '.claude\skills'
$tmp = Join-Path ([IO.Path]::GetTempPath()) ('b24-' + [guid]::NewGuid().ToString('N'))

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host 'Скачиваю b24 и скилл…'
    Invoke-WebRequest "$base/$exeName" -OutFile "$tmp\b24.exe" -UseBasicParsing
    Invoke-WebRequest "$base/bitrix24-skill.zip" -OutFile "$tmp\skill.zip" -UseBasicParsing
    Invoke-WebRequest "$base/checksums.txt" -OutFile "$tmp\checksums.txt" -UseBasicParsing

    $sums = Get-Content "$tmp\checksums.txt"
    foreach ($pair in @(@("$tmp\b24.exe", $exeName), @("$tmp\skill.zip", 'bitrix24-skill.zip'))) {
        $want = ($sums | Where-Object { $_ -match (' ' + [regex]::Escape($pair[1]) + '$') } | Select-Object -First 1) -replace ' .*$', ''
        $got = (Get-FileHash -Algorithm SHA256 $pair[0]).Hash.ToLower()
        if (-not $want -or $want -ne $got) {
            throw "Контрольная сумма $($pair[1]) не сошлась — файл повреждён или подменён, установка остановлена."
        }
    }

    New-Item -ItemType Directory -Force -Path $binDir | Out-Null
    Copy-Item "$tmp\b24.exe" (Join-Path $binDir 'b24.exe') -Force
    Write-Host "✓ b24 → $binDir\b24.exe"

    New-Item -ItemType Directory -Force -Path $skills | Out-Null
    $skillDir = Join-Path $skills 'bitrix24'
    if (Test-Path $skillDir) { Remove-Item -Recurse -Force $skillDir }
    Expand-Archive -Path "$tmp\skill.zip" -DestinationPath $skills -Force
    Write-Host "✓ скилл → $skillDir"

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not (($userPath -split ';') -contains $binDir)) {
        $newPath = if ($userPath) { "$userPath;$binDir" } else { $binDir }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-Host "✓ $binDir добавлен в PATH — откройте новое окно терминала"
    }
    $env:Path = "$env:Path;$binDir"

    & (Join-Path $binDir 'b24.exe') setup
    if ($LASTEXITCODE -ne 0) { Write-Host 'MCP не подключён: выполните позже b24 setup' }
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

Write-Host @'

Дальше:
  1. В Битриксе: Приложения → Разработчикам → Готовые сценарии → Другое → Входящий вебхук.
     Отметьте нужные права, сохраните, скопируйте ссылку «Вебхук для вызова rest api».
  2. В терминале: b24 add work --clipboard   (ссылку в чат с Claude не вставлять)
  3. Откройте новый разговор с Claude — он увидит портал через MCP.
'@
