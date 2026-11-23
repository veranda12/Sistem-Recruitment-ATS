<#
.SYNOPSIS
  Stack demo untuk presentasi: database terpisah ats_demo_dev berisi data contoh lengkap, frontend build produksi,
  dan opsi alamat publik (mis. ngrok). Tidak menyentuh stack dev (:8080/:5173) maupun database di .env.

  Port: web :4173 (arahkan tunnel ke sini), API :8100, worker :8101, worker CV :8102.
  Data bertahan di antara sesi. -Reset menghapus dan mengisi ulang data demo dari awal.

.PARAMETER PublicOrigin
  Alamat yang dibuka penonton, mis. https://nama-anda.ngrok-free.app. Bawaan http://localhost:4173.
  Login hanya diterima dari alamat ini (pemeriksaan Origin), jadi buka aplikasi lewat alamat yang sama.
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File ops\windows\demo.ps1 start -Reset
  powershell -ExecutionPolicy Bypass -File ops\windows\demo.ps1 start -PublicOrigin https://nama-anda.ngrok-free.app
  ngrok http --url=nama-anda.ngrok-free.app 4173
  powershell -ExecutionPolicy Bypass -File ops\windows\demo.ps1 stop
#>
param(
    [Parameter(Position = 0)][ValidateSet('start', 'stop')][string]$Action = 'start',
    [string]$PublicOrigin = 'http://localhost:4173',
    [switch]$Reset
)
. "$PSScriptRoot\common.ps1"

$DemoDir = Join-Path $DataDir 'demo'
$PidFile = Join-Path $DemoDir 'pids.json'
$DemoDb = 'ats_demo_dev'

function Stop-Demo {
    if (-not (Test-Path $PidFile)) { Write-Ok 'Stack demo tidak berjalan.'; return }
    $ids = Get-Content -LiteralPath $PidFile -Raw | ConvertFrom-Json
    foreach ($p in $ids.PSObject.Properties) {
        & taskkill.exe /PID $p.Value /T /F 2>&1 | Out-Null
        Write-Ok "Berhenti: $($p.Name) (PID $($p.Value))"
    }
    Remove-Item -LiteralPath $PidFile
}

if ($Action -eq 'stop') { Stop-Demo; return }
if (Test-Path $PidFile) { throw 'Stack demo masih berjalan. Jalankan: demo.ps1 stop' }
foreach ($port in 4173, 8100, 8101, 8102) {
    if ((Test-TcpPort '127.0.0.1' $port) -or (Test-TcpPort '::1' $port)) { throw "Port $port sudah dipakai proses lain." }
}
$PublicOrigin = $PublicOrigin.TrimEnd('/')
$publicHost = ([uri]$PublicOrigin).Host

Import-DotEnv
if ([string]::IsNullOrWhiteSpace($env:SEED_PASSWORD)) { throw 'SEED_PASSWORD di .env wajib diisi: dipakai sebagai kata sandi semua akun demo.' }
# Semua override hanya untuk proses ini dan anak-anaknya; .env tidak diubah.
$env:DATABASE_URL = [regex]::Replace($env:DATABASE_URL, '^(postgres(?:ql)?://[^/]+/)[^?]+', "`${1}$DemoDb")
$env:HTTP_ADDR = '127.0.0.1:8100'
$env:PUBLIC_ORIGIN = $PublicOrigin
$env:WORKER_HEALTH_ADDR = '127.0.0.1:8101'
$env:CV_WORKER_HEALTH_ADDR = '127.0.0.1:8102'
$env:METRICS_ADDR = ''
$env:CV_WORKER_METRICS_ADDR = ''
$env:WORKER_METRICS_ADDR = ''
# Tunnel meneruskan dari agen lokal; IP asli pengunjung dibaca dari X-Forwarded-For.
$env:TRUSTED_PROXIES = '127.0.0.1,::1'
$env:REDIS_URL = ''
$env:MAIL_DIR = Join-Path $DemoDir 'mail'
$env:OBJECT_DIR = Join-Path $DemoDir 'objects'
$env:CV_WORK_DIR = Join-Path $DemoDir 'cvwork'

Write-Step "Database $DemoDb"
$db = Get-DbParts $env:DATABASE_URL
$env:PGPASSWORD = $db.Pass
$exists = & psql -h $db.Host -p $db.Port -U $db.User -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = '$DemoDb'"
$fresh = $Reset
if ("$exists".Trim() -ne '1') {
    & createdb -h $db.Host -p $db.Port -U $db.User -E UTF8 -T template0 $DemoDb
    if ($LASTEXITCODE -ne 0) { throw "Gagal membuat database $DemoDb" }
    Write-Ok "Dibuat: $DemoDb"
    $fresh = $true
}
Remove-Item Env:PGPASSWORD
$bin = Join-Path $Root 'backend\bin'
if ($fresh) {
    foreach ($d in 'mail', 'objects', 'cvwork') {
        $p = Join-Path $DemoDir $d
        if (Test-Path $p) { Remove-Item -Recurse -Force $p }
    }
    # seed --reset hanya mau berjalan pada database berakhiran _dev/_test/_e2e/_load.
    Invoke-Checked (Join-Path $bin "seed$Exe") @('--reset')
    Write-Ok 'Skema dibuat dan data dasar PT Arunika Daya Persada (fiktif) diisi.'
} else {
    Invoke-Checked (Join-Path $bin "migrate$Exe") @('up')
    Write-Ok 'Data demo sebelumnya dipakai (tambahkan -Reset untuk mengisi ulang dari awal).'
}
foreach ($d in 'mail', 'objects', 'cvwork', 'logs') { New-Item -ItemType Directory -Force (Join-Path $DemoDir $d) | Out-Null }

Write-Step 'Build frontend produksi'
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
$env:VITE_API_TARGET = 'http://127.0.0.1:8100'
$env:VITE_ALLOWED_HOSTS = $publicHost
$procs.web = Start-Process -FilePath 'npx.cmd' -ArgumentList 'vite', 'preview', '--outDir', $web, '--port', '4173', '--strictPort' `
    -WorkingDirectory $front -WindowStyle Hidden -PassThru `
    -RedirectStandardOutput (Join-Path $logs 'web.log') -RedirectStandardError (Join-Path $logs 'web.err.log')
$ids = [ordered]@{}
foreach ($k in $procs.Keys) { $ids[$k] = $procs[$k].Id }
$ids | ConvertTo-Json | Set-Content -LiteralPath $PidFile -Encoding UTF8

$deadline = (Get-Date).AddSeconds(40)
$ready = $false
do {
    Start-Sleep -Milliseconds 500
    # Vite preview mendengarkan pada "localhost" (bisa ::1 saja), jadi diperiksa lewat HTTP.
    try {
        Invoke-RestMethod -Uri 'http://127.0.0.1:8100/readyz' -TimeoutSec 2 | Out-Null
        Invoke-WebRequest -Uri 'http://localhost:4173/' -UseBasicParsing -TimeoutSec 2 | Out-Null
        $ready = $true
    } catch { $ready = $false }
    foreach ($k in 'api', 'worker', 'web') { if ($procs[$k].HasExited) { Stop-Demo; throw "Proses $k berhenti. Lihat $logs" } }
} while (-not $ready -and (Get-Date) -lt $deadline)
if (-not $ready) { Stop-Demo; throw "Stack demo tidak siap dalam 40 detik. Lihat $logs" }
if ($procs.cvworker.HasExited) { Write-Warn2 "Worker CV berhenti (alat CV belum lengkap?). Fitur lain tetap jalan. Lihat $logs\cvworker.log" }

if ($fresh) {
    Write-Step 'Mengisi data demo lewat API (kandidat, lamaran, ujian, interview, persetujuan)'
    $env:API = 'http://127.0.0.1:8100/api/v1'
    $env:ORIGIN = $PublicOrigin
    $env:PGUSER = $db.User
    $env:PGPASSWORD = $db.Pass
    $env:PGDATABASE = $DemoDb
    Invoke-Checked 'node' @((Join-Path $Root 'tests\demo\demo-data.mjs'))
    Invoke-Checked 'node' @((Join-Path $Root 'tests\demo\demo-accounts.mjs'))
    Remove-Item Env:PGPASSWORD
}

Write-Host ''
Write-Ok "Demo siap: $PublicOrigin  (lokal: http://localhost:4173)"
Write-Host "  Kata sandi semua akun = SEED_PASSWORD di .env. Daftar akun: docs\DEMO.md" -ForegroundColor Gray
if ($PublicOrigin -notlike 'http://localhost*') {
    Write-Host "  Jalankan tunnel: ngrok http --url=$publicHost 4173" -ForegroundColor Gray
    Write-Host '  Login hanya diterima dari alamat publik di atas; buka juga lewat alamat itu di laptop Anda.' -ForegroundColor Gray
}
