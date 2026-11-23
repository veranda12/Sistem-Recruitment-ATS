<#
.SYNOPSIS
  Menjalankan API, worker, worker CV, dan server dev frontend di latar belakang. Log di .data\logs, PID di .data\pids.json.
.EXAMPLE
  powershell -File ops\windows\start.ps1
#>
param([switch]$NoFrontend)
. "$PSScriptRoot\common.ps1"
Import-DotEnv

$logs = Join-Path $DataDir 'logs'
New-Item -ItemType Directory -Force $logs | Out-Null
$pidFile = Join-Path $DataDir 'pids.json'
if (Test-Path $pidFile) { throw 'Proses sebelumnya mungkin masih berjalan. Jalankan stop.ps1 dulu.' }

$addr = if ($env:HTTP_ADDR) { $env:HTTP_ADDR } else { '127.0.0.1:8080' }
$apiPort = [int]($addr -replace '^.*:', '')
if (Test-TcpPort '127.0.0.1' $apiPort 300) {
    throw "Port $apiPort (HTTP_ADDR) sudah dipakai aplikasi lain. Ganti HTTP_ADDR di .env, mis. HTTP_ADDR=127.0.0.1:8090, lalu jalankan lagi."
}
# Proxy /api di Vite dev mengikuti port API dari .env.
$env:VITE_API_TARGET = "http://$addr"

$bin = Join-Path $Root 'backend\bin'
$procs = @{}
$procs.api = Start-Process -FilePath (Join-Path $bin "api$Exe") -WorkingDirectory $Root -WindowStyle Hidden -PassThru `
    -RedirectStandardOutput (Join-Path $logs 'api.log') -RedirectStandardError (Join-Path $logs 'api.err.log')
$procs.worker = Start-Process -FilePath (Join-Path $bin "worker$Exe") -WorkingDirectory $Root -WindowStyle Hidden -PassThru `
    -RedirectStandardOutput (Join-Path $logs 'worker.log') -RedirectStandardError (Join-Path $logs 'worker.err.log')
# Worker CV terpisah (antrean dan sumber daya sendiri). Redis opsional: tanpa Redis, job diambil dari PostgreSQL.
if (Test-Path (Join-Path $bin "cvworker$Exe")) {
    $procs.cvworker = Start-Process -FilePath (Join-Path $bin "cvworker$Exe") -WorkingDirectory $Root -WindowStyle Hidden -PassThru `
        -RedirectStandardOutput (Join-Path $logs 'cvworker.log') -RedirectStandardError (Join-Path $logs 'cvworker.err.log')
}
if (-not $NoFrontend) {
    $procs.web = Start-Process -FilePath 'npm.cmd' -ArgumentList 'run', 'dev' -WorkingDirectory (Join-Path $Root 'frontend') -WindowStyle Hidden -PassThru `
        -RedirectStandardOutput (Join-Path $logs 'web.log') -RedirectStandardError (Join-Path $logs 'web.err.log')
}
$ids = @{}
foreach ($k in $procs.Keys) { $ids[$k] = $procs[$k].Id }
$ids | ConvertTo-Json | Set-Content -LiteralPath $pidFile -Encoding UTF8

# Tunggu readiness API (maks 30 detik).
$deadline = (Get-Date).AddSeconds(30)
do {
    Start-Sleep -Milliseconds 500
    try { $r = Invoke-RestMethod -Uri "http://$addr/readyz" -TimeoutSec 2; break } catch { $r = $null }
    if ($procs.api.HasExited) { throw "API berhenti. Lihat $logs\api.err.log dan api.log" }
} while ((Get-Date) -lt $deadline)
if (-not $r) { throw "API tidak siap dalam 30 detik. Lihat $logs\api.log" }
Write-Ok "API siap di http://$addr (database: $($r.dependencies.database), redis: $($r.dependencies.redis))"
Write-Ok "Worker PID $($procs.worker.Id)"
if ($procs.cvworker) {
    Start-Sleep -Seconds 2
    if ($procs.cvworker.HasExited) { Write-Warn2 "Worker CV berhenti (preflight gagal?). Lihat $logs\cvworker.log; jalankan ops\windows\cv-check.ps1 -Mode preflight" }
    else { Write-Ok "Worker CV PID $($procs.cvworker.Id) (Redis: $(if ($env:REDIS_URL) { 'dipakai' } else { 'tidak, antrean via PostgreSQL' }))" }
}
if (-not $NoFrontend) { Write-Ok "Frontend dev: $env:PUBLIC_ORIGIN (menunggu Vite beberapa detik)" }
