<#
.SYNOPSIS
  Memeriksa prasyarat pengembangan di Windows dan menyebut setiap dependency yang gagal.
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File ops\windows\preflight.ps1
#>
. "$PSScriptRoot\common.ps1"
$failed = 0

# -Optional: tool yang tidak dibutuhkan untuk menjalankan aplikasi; ketiadaannya hanya peringatan.
function Check-Version([string]$name, [string]$cmd, [string[]]$argsList, [string]$pattern, [version]$min, [string]$hint, [switch]$Optional) {
    if (-not (Test-Command $cmd)) {
        if ($Optional) { Write-Warn2 "$name tidak ditemukan di PATH (opsional). $hint"; return }
        Write-Fail "$name tidak ditemukan di PATH. $hint"; $script:failed++; return
    }
    $out = (& $cmd @argsList 2>&1 | Out-String)
    $m = [regex]::Match($out, $pattern)
    if (-not $m.Success) { Write-Warn2 "$name terpasang, versi tidak terbaca: $($out.Trim())"; return }
    $v = [version]$m.Groups[1].Value
    if ($v -lt $min) { Write-Fail "$name $v terlalu lama (minimal $min). $hint"; $script:failed++ } else { Write-Ok "$name $v" }
}

Write-Step 'Tool'
Check-Version 'Go' 'go' @('version') 'go(\d+\.\d+(?:\.\d+)?)' ([version]'1.24') 'Pasang versi terbaru dari https://go.dev/dl/ (go.mod meminta 1.26; Go 1.24/1.25 mengunduh toolchain itu otomatis saat build).'
Check-Version 'Node.js' 'node' @('--version') 'v(\d+\.\d+\.\d+)' ([version]'20.19') 'Pasang Node LTS dari https://nodejs.org/'
Check-Version 'npm' 'npm.cmd' @('--version') '(\d+\.\d+\.\d+)' ([version]'10.0') 'Terpasang bersama Node.js.'
Check-Version 'psql' 'psql' @('--version') '(\d+\.\d+)' ([version]'17.0') 'Tambahkan "C:\Program Files\PostgreSQL\17\bin" ke PATH.'
Check-Version 'sqlc (hanya untuk mengubah query)' 'sqlc' @('version') 'v(\d+\.\d+\.\d+)' ([version]'1.29.0') 'Hanya untuk mengubah query: unduh sqlc_1.29.0_windows_amd64.zip dari GitHub sqlc-dev/sqlc, taruh di PATH.' -Optional

Write-Step 'Konfigurasi'
if (-not (Test-Path $EnvFile)) {
    Write-Fail ".env belum ada. Jalankan: powershell -File ops\windows\new-env.ps1 -DatabaseUrl 'postgres://USER:SANDI@localhost:5432/ats_dev?sslmode=disable'"
    $failed++
} else {
    Import-DotEnv
    foreach ($k in 'DATABASE_URL', 'SESSION_KEY', 'NIK_ENC_KEY', 'NIK_INDEX_KEY', 'PUBLIC_ORIGIN') {
        if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($k))) { Write-Fail "$k kosong di .env"; $failed++ } else { Write-Ok "$k terisi" }
    }
}

Write-Step 'Layanan'
if ($env:DATABASE_URL) {
    $db = Get-DbParts $env:DATABASE_URL
    if (Test-TcpPort $db.Host ([int]$db.Port)) {
        Write-Ok "PostgreSQL menerima koneksi di $($db.Host):$($db.Port)"
        if (Test-Command 'psql') {
            $env:PGPASSWORD = $db.Pass
            $ver = & psql -h $db.Host -p $db.Port -U $db.User -d postgres -tAc 'show server_version' 2>&1
            if ($LASTEXITCODE -ne 0) { Write-Fail "Login PostgreSQL sebagai '$($db.User)' gagal: $ver"; $failed++ }
            else {
                Write-Ok "Login PostgreSQL sebagai '$($db.User)' (server $($ver.Trim()))"
                $exists = & psql -h $db.Host -p $db.Port -U $db.User -d postgres -tAc "select 1 from pg_database where datname='$($db.Db)'"
                if ("$exists".Trim() -eq '1') { Write-Ok "Database $($db.Db) ada" } else { Write-Warn2 "Database $($db.Db) belum ada; setup.ps1 akan membuatnya." }
            }
            Remove-Item Env:\PGPASSWORD -ErrorAction SilentlyContinue
        }
    } else {
        Write-Fail "PostgreSQL tidak dapat dihubungi di $($db.Host):$($db.Port). Jalankan layanan: Start-Service postgresql-x64-17"
        $failed++
    }
}
if ($env:REDIS_URL) {
    $m = [regex]::Match($env:REDIS_URL, '^rediss?://(?:[^@]*@)?(?<h>[^:/]+)(?::(?<p>\d+))?')
    $rp = if ($m.Groups['p'].Success) { [int]$m.Groups['p'].Value } else { 6379 }
    if (Test-TcpPort $m.Groups['h'].Value $rp) { Write-Ok "Redis terjangkau di $($m.Groups['h'].Value):$rp" }
    else { Write-Warn2 "Redis di $($m.Groups['h'].Value):$rp tidak terjangkau. API tetap jalan dalam mode degraded (cache mati, limiter lokal, ekspor data pribadi ditolak)." }
} else {
    Write-Warn2 'REDIS_URL kosong: mode degraded (cache publik mati, limiter lokal, ekspor data pribadi ditolak). Opsional: ops\windows\cv-tools.ps1 lalu redis-start.ps1 (docs\operations\windows-dev.md).'
}

Write-Step 'Port lokal'
# Port mengikuti .env (HTTP_ADDR, WORKER_HEALTH_ADDR); bawaan 8080 dan 8081.
function Port-Of([string]$v, [int]$d) { if ($v) { [int]($v -replace '^.*:', '') } else { $d } }
$ports = [ordered]@{ 'API (HTTP_ADDR)' = (Port-Of $env:HTTP_ADDR 8080); 'worker (WORKER_HEALTH_ADDR)' = (Port-Of $env:WORKER_HEALTH_ADDR 8081); 'frontend dev' = 5173 }
foreach ($k in $ports.Keys) {
    $p = $ports[$k]
    if (Test-TcpPort '127.0.0.1' $p 300) {
        Write-Warn2 "Port $p untuk $k sudah dipakai proses lain. Bila itu instance ATS sebelumnya jalankan stop.ps1; bila aplikasi lain, ganti port di .env (mis. HTTP_ADDR=127.0.0.1:8090)."
    } else { Write-Ok "Port $p ($k) bebas" }
}

Write-Host ''
if ($failed -gt 0) { Write-Host "Preflight gagal: $failed masalah perlu diperbaiki." -ForegroundColor Red; exit 1 }
Write-Host 'Preflight lulus.' -ForegroundColor Green
