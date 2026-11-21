<#
.SYNOPSIS
  Setup dari checkout bersih: dependency, build, buat database bila belum ada, migrasi, seed (opsional).
.PARAMETER SkipSeed
  Lewati seed data fiktif (wajib untuk instalasi yang bukan dev/staging).
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File ops\windows\setup.ps1
#>
param([switch]$SkipSeed, [switch]$SkipFrontend)
. "$PSScriptRoot\common.ps1"

& "$PSScriptRoot\preflight.ps1"
if ($LASTEXITCODE -ne 0) { throw 'Perbaiki temuan preflight terlebih dahulu.' }
Import-DotEnv

Write-Step 'Dependency Go'
Invoke-Checked 'go' @('mod', 'download') (Join-Path $Root 'backend')

Write-Step 'Build binary backend (api, worker, migrate, seed)'
$bin = Join-Path $Root 'backend\bin'
New-Item -ItemType Directory -Force $bin | Out-Null
Invoke-Checked 'go' @('build', '-o', $bin, './cmd/...') (Join-Path $Root 'backend')

if (-not $SkipFrontend) {
    Write-Step 'Dependency frontend (npm ci dari lockfile)'
    Invoke-Checked 'npm.cmd' @('ci', '--no-audit', '--no-fund') (Join-Path $Root 'frontend')
}

Write-Step 'Database'
$db = Get-DbParts $env:DATABASE_URL
$env:PGPASSWORD = $db.Pass
try {
    $exists = & psql -h $db.Host -p $db.Port -U $db.User -d postgres -tAc "select 1 from pg_database where datname='$($db.Db)'"
    if ("$exists".Trim() -ne '1') {
        & createdb -h $db.Host -p $db.Port -U $db.User -E UTF8 -T template0 $db.Db
        if ($LASTEXITCODE -ne 0) { throw "Gagal membuat database $($db.Db)." }
        Write-Ok "Database $($db.Db) dibuat"
    } else { Write-Ok "Database $($db.Db) sudah ada" }
} finally { Remove-Item Env:\PGPASSWORD -ErrorAction SilentlyContinue }

Write-Step 'Migrasi'
Invoke-Checked (Join-Path $bin "migrate$Exe") @('up')

if (-not $SkipSeed) {
    if ([string]::IsNullOrWhiteSpace($env:SEED_PASSWORD)) { Write-Warn2 'SEED_PASSWORD kosong: seed dilewati.' }
    else {
        Write-Step 'Seed data fiktif'
        Invoke-Checked (Join-Path $bin "seed$Exe") @()
    }
}
New-Item -ItemType Directory -Force (Join-Path $DataDir 'logs'), (Join-Path $DataDir 'mail'), (Join-Path $DataDir 'objects') | Out-Null
Write-Host ''
Write-Host 'Setup selesai. Jalankan: powershell -File ops\windows\start.ps1' -ForegroundColor Green
