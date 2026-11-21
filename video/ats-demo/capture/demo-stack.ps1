<#
.SYNOPSIS
  Stack ATS khusus rekaman video: database ats_video_dev, API :8090, worker :8091, worker CV :8092,
  frontend build produksi :5174. Tidak menyentuh stack dev biasa (:8080/:5173) maupun database-nya.
.EXAMPLE
  powershell -File video\ats-demo\capture\demo-stack.ps1 start    # reset DB demo + seed + jalankan
  powershell -File video\ats-demo\capture\demo-stack.ps1 stop
#>
param([ValidateSet('start', 'stop')][string]$Action = 'start')
. "$PSScriptRoot\..\..\..\ops\windows\common.ps1"

$DemoDir = Join-Path $DataDir 'video-demo'
$PidFile = Join-Path $DemoDir 'pids.json'
$DemoDb = 'ats_video_dev'

function Stop-Demo {
    if (-not (Test-Path $PidFile)) { Write-Ok 'Stack demo tidak berjalan.'; return }
    $ids = Get-Content -LiteralPath $PidFile -Raw | ConvertFrom-Json
    foreach ($p in $ids.PSObject.Properties) {
        # /T ikut menghentikan proses anak (npx -> node).
        & taskkill.exe /PID $p.Value /T /F 2>&1 | Out-Null
        Write-Ok "Berhenti: $($p.Name) (PID $($p.Value))"
    }
    Remove-Item -LiteralPath $PidFile
}

if ($Action -eq 'stop') { Stop-Demo; return }
if (Test-Path $PidFile) { throw "Stack demo masih berjalan. Jalankan: demo-stack.ps1 stop" }
foreach ($port in 8090, 8091, 8092, 5174) {
    if ((Test-TcpPort '127.0.0.1' $port) -or (Test-TcpPort '::1' $port)) { throw "Port $port sudah dipakai proses lain." }
}

Import-DotEnv
# Semua override berlaku untuk proses ini dan anak-anaknya saja; .env tidak diubah.
$env:DATABASE_URL = [regex]::Replace($env:DATABASE_URL, '^(postgres(?:ql)?://[^/]+/)[^?]+', "`${1}$DemoDb")
$env:HTTP_ADDR = '127.0.0.1:8090'
$env:PUBLIC_ORIGIN = 'http://localhost:5174'
$env:WORKER_HEALTH_ADDR = '127.0.0.1:8091'
$env:CV_WORKER_HEALTH_ADDR = '127.0.0.1:8092'
$env:METRICS_ADDR = ''
$env:CV_WORKER_METRICS_ADDR = ''
# Tanpa Redis: antrean CV lewat PostgreSQL, sehingga tidak bercampur dengan worker CV stack dev.
$env:REDIS_URL = ''
$env:MAIL_DIR = Join-Path $DemoDir 'mail'
$env:OBJECT_DIR = Join-Path $DemoDir 'objects'
$env:CV_WORK_DIR = Join-Path $DemoDir 'cvwork'

# Data demo lama dibuang agar rekaman dapat diulang tanpa lamaran ganda.
foreach ($d in 'mail', 'objects', 'cvwork', 'logs') {
    $p = Join-Path $DemoDir $d
    if (Test-Path $p) { Remove-Item -Recurse -Force $p }
    New-Item -ItemType Directory -Force $p | Out-Null
}

Write-Step "Database $DemoDb"
$db = Get-DbParts $env:DATABASE_URL
$env:PGPASSWORD = $db.Pass
$exists = & psql -h $db.Host -p $db.Port -U $db.User -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = '$DemoDb'"
if ("$exists".Trim() -ne '1') {
    & psql -h $db.Host -p $db.Port -U $db.User -d postgres -c "CREATE DATABASE $DemoDb" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Gagal membuat database $DemoDb" }
    Write-Ok "Dibuat: $DemoDb"
}
Remove-Item Env:PGPASSWORD
# seed --reset hanya mau berjalan pada database berakhiran _dev/_test/_e2e/_load.
$bin = Join-Path $Root 'backend\bin'
Invoke-Checked (Join-Path $bin "seed$Exe") @('--reset')
Write-Ok 'Skema direset dan data fiktif PT Arunika Daya Persada diisi.'

Write-Step 'Build frontend (produksi) untuk rekaman'
$web = Join-Path $DemoDir 'web'
$front = Join-Path $Root 'frontend'
Invoke-Checked 'npx.cmd' @('vite', 'build', '--outDir', $web, '--emptyOutDir', '--logLevel', 'warn') $front

Write-Step 'Menjalankan proses'
$logs = Join-Path $DemoDir 'logs'
$procs = [ordered]@{}
foreach ($name in 'api', 'worker', 'cvworker') {
    $procs[$name] = Start-Process -FilePath (Join-Path $bin "$name$Exe") -WorkingDirectory $Root -WindowStyle Hidden -PassThru `
        -RedirectStandardOutput (Join-Path $logs "$name.log") -RedirectStandardError (Join-Path $logs "$name.err.log")
}
$env:VITE_API_TARGET = 'http://127.0.0.1:8090'
$procs.web = Start-Process -FilePath 'npx.cmd' -ArgumentList 'vite', 'preview', '--outDir', $web, '--port', '5174', '--strictPort' `
    -WorkingDirectory $front -WindowStyle Hidden -PassThru `
    -RedirectStandardOutput (Join-Path $logs 'web.log') -RedirectStandardError (Join-Path $logs 'web.err.log')
$ids = [ordered]@{}
foreach ($k in $procs.Keys) { $ids[$k] = $procs[$k].Id }
$ids | ConvertTo-Json | Set-Content -LiteralPath $PidFile -Encoding UTF8

$deadline = (Get-Date).AddSeconds(40)
$ready = $false
do {
    Start-Sleep -Milliseconds 500
    # Vite preview mendengarkan pada "localhost" (bisa ::1 saja), jadi diperiksa lewat HTTP, bukan TCP 127.0.0.1.
    try {
        Invoke-RestMethod -Uri 'http://127.0.0.1:8090/readyz' -TimeoutSec 2 | Out-Null
        Invoke-WebRequest -Uri 'http://localhost:5174/' -UseBasicParsing -TimeoutSec 2 | Out-Null
        $ready = $true
    } catch { $ready = $false }
    foreach ($k in $procs.Keys) { if ($procs[$k].HasExited) { Stop-Demo; throw "Proses $k berhenti. Lihat $logs" } }
} while (-not $ready -and (Get-Date) -lt $deadline)
if (-not $ready) { Stop-Demo; throw "Stack demo tidak siap dalam 40 detik. Lihat $logs" }
Write-Ok 'Stack demo siap: http://localhost:5174 (API :8090, database ats_video_dev)'
