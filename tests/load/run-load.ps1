<#
.SYNOPSIS
  Uji beban ujian end-to-end di mesin lokal/staging yang disetujui: reset DB *_load, seed, peserta sintetis,
  jalankan API + worker, k6, rekonsiliasi SQL, lalu catat konfigurasi dan hasil ke tests\load\results\<run>.
  JANGAN arahkan ke produksi atau layanan eksternal.
.EXAMPLE
  powershell -File tests\load\run-load.ps1 -Profile smoke -Participants 5 -ExamSeconds 60
  powershell -File tests\load\run-load.ps1 -Profile baseline -Participants 100 -ExamSeconds 180
#>
param(
    [ValidateSet('smoke', 'baseline', 'ramp', 'burst-login', 'burst-submit')][string]$Profile = 'smoke',
    [int]$Participants = 5,
    [int]$ExamSeconds = 60,
    [string]$DatabaseUrl = 'postgres://pos:pos@localhost:5432/ats_load?sslmode=disable',
    [int]$DbMaxConns = 20,
    [int]$Seed = 20261121
)
. "$PSScriptRoot\..\..\ops\windows\common.ps1"
$run = 'r' + (Get-Date).ToString('MMddHHmmss')
$resDir = Join-Path $Root "tests\load\results\$run"
New-Item -ItemType Directory -Force $resDir | Out-Null
$db = Get-DbParts $DatabaseUrl
if ($db.Db -notlike '*_load') { throw 'Database uji beban harus berakhiran _load.' }

# Lingkungan uji beban (kunci khusus run, bukan kunci instalasi).
function NewKey { $b = New-Object byte[] 32; [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); [Convert]::ToBase64String($b) }
$envs = @{
    APP_ENV = 'test'; HTTP_ADDR = '127.0.0.1:8090'; PUBLIC_ORIGIN = 'http://localhost:5173'; DATABASE_URL = $DatabaseUrl; ORG_SLUG = 'arunika'
    SESSION_KEY = (NewKey); NIK_ENC_KEY = (NewKey); NIK_INDEX_KEY = (NewKey); SEED_PASSWORD = 'Beban-Uji-2026'; LOG_LEVEL = 'warn'
    DB_MAX_CONNS = "$DbMaxConns"; WORKER_HEALTH_ADDR = '127.0.0.1:8091'; METRICS_ADDR = '127.0.0.1:19090'; MAIL_DIR = (Join-Path $resDir 'mail')
    # Generator k6 memakai satu IP; kuota limiter per IP dikalikan agar tidak mengukur limiter. Bukan konfigurasi produksi.
    RATE_LIMIT_SCALE = '100'
}
foreach ($k in $envs.Keys) { [Environment]::SetEnvironmentVariable($k, $envs[$k], 'Process') }
$env:PGPASSWORD = $db.Pass
$exists = & psql -h $db.Host -p $db.Port -U $db.User -d postgres -tAc "select 1 from pg_database where datname='$($db.Db)'"
if ("$exists".Trim() -ne '1') { & createdb -h $db.Host -p $db.Port -U $db.User -E UTF8 -T template0 $db.Db }

$bin = Join-Path $Root 'backend\bin'
Write-Step "Reset $($db.Db), seed, $Participants peserta sintetis (run $run)"
Invoke-Checked (Join-Path $bin "seed$Exe") @('--reset')
$fixture = Join-Path $resDir 'fixture.json'
Invoke-Checked (Join-Path $bin "loadseed$Exe") @('-run', $run, '-n', "$Participants", '-out', $fixture, '-minutes', "$([math]::Max(30, [math]::Ceiling($ExamSeconds / 60) + 15))")

Write-Step 'Jalankan API dan worker'
$api = Start-Process (Join-Path $bin "api$Exe") -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $resDir 'api.log') -RedirectStandardError (Join-Path $resDir 'api.err.log')
$wrk = Start-Process (Join-Path $bin "worker$Exe") -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $resDir 'worker.log') -RedirectStandardError (Join-Path $resDir 'worker.err.log')
try {
    $ok = $false
    for ($i = 0; $i -lt 40 -and -not $ok; $i++) { Start-Sleep -Milliseconds 500; try { Invoke-RestMethod http://127.0.0.1:8090/readyz -TimeoutSec 2 | Out-Null; $ok = $true } catch {} }
    if (-not $ok) { throw 'API tidak siap' }

    $cpu = (Get-CimInstance Win32_Processor | Select-Object -First 1)
    $mem = [math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory / 1GB, 1)
    $pgv = (& psql -h $db.Host -p $db.Port -U $db.User -d $db.Db -tAc 'show server_version').Trim()
    $maxc = (& psql -h $db.Host -p $db.Port -U $db.User -d $db.Db -tAc 'show max_connections').Trim()
    $commit = (& git -C $Root rev-parse --short HEAD).Trim()
    $info = [ordered]@{ run = $run; profile = $Profile; participants = $Participants; examSeconds = $ExamSeconds; seed = $Seed; commit = $commit
        startedAt = (Get-Date).ToUniversalTime().ToString('o'); cpu = "$($cpu.Name) ($($cpu.NumberOfCores) core / $($cpu.NumberOfLogicalProcessors) thread)"; ramGB = $mem
        os = (Get-CimInstance Win32_OperatingSystem).Caption; postgres = $pgv; maxConnections = $maxc; dbMaxConnsApi = $DbMaxConns
        topology = 'API, worker, PostgreSQL, dan generator k6 di satu laptop (bukan staging terpisah)'; redis = 'tidak ada (mode degraded)' }

    # Sampling sumber daya tiap 15 detik: CPU (detik kumulatif) dan working set per proses.
    $samplesFile = Join-Path $resDir 'resources.csv'
    $sampler = Start-Job -ArgumentList $samplesFile -ScriptBlock {
        param($f)
        'time,process,cpu_seconds,ws_mb' | Set-Content $f
        while ($true) {
            $t = (Get-Date).ToString('o')
            foreach ($n in 'api', 'worker', 'postgres', 'k6') {
                $ps = Get-Process -Name $n -ErrorAction SilentlyContinue
                if ($ps) { "$t,$n,$([math]::Round(($ps | Measure-Object CPU -Sum).Sum,1)),$([math]::Round(($ps | Measure-Object WorkingSet64 -Sum).Sum/1MB,0))" | Add-Content $f }
            }
            Start-Sleep -Seconds 15
        }
    }
    Write-Step "k6 profil $Profile"
    $k6args = @('run', '--quiet', '-e', 'BASE=http://127.0.0.1:8090', '-e', 'ORIGIN=http://localhost:5173', '-e', "FIXTURE=$fixture", '-e', "PASSWORD=$($envs.SEED_PASSWORD)",
        '-e', "PROFILE=$Profile", '-e', "EXAM_SECONDS=$ExamSeconds", '-e', "SEED=$Seed", '-e', "VUS=$Participants", '-e', "SUMMARY=$(Join-Path $resDir 'k6-summary.json')")
    if ($Profile -eq 'burst-submit') { $k6args += @('-e', "SUBMIT_AT=$([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds() + ($ExamSeconds + 20) * 1000)") }
    $k6args += (Join-Path $Root 'tests\load\exam.js')
    & k6 @k6args 2>&1 | Tee-Object -FilePath (Join-Path $resDir 'k6-stdout.txt')
    $info['k6ExitCode'] = $LASTEXITCODE
    $info['finishedAt'] = (Get-Date).ToUniversalTime().ToString('o')
    try { (Invoke-WebRequest http://127.0.0.1:19090/metrics -UseBasicParsing).Content | Set-Content (Join-Path $resDir 'api-metrics.txt') } catch {}
} finally {
    if ($sampler) { Stop-Job $sampler; Remove-Job $sampler -Force }
    foreach ($p in $api, $wrk) { if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force } }
}

Write-Step 'Rekonsiliasi SQL (read-only)'
& psql -h $db.Host -p $db.Port -U $db.User -d $db.Db -v run=$run -v seed=$Seed -f (Join-Path $Root 'tests\load\reconcile.sql') 2>&1 | Tee-Object -FilePath (Join-Path $resDir 'reconcile.txt')
Remove-Item Env:\PGPASSWORD -ErrorAction SilentlyContinue
$info | ConvertTo-Json | Set-Content -Encoding UTF8 (Join-Path $resDir 'run.json')
Write-Ok "Hasil di $resDir"
