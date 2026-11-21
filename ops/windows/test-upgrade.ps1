<#
.SYNOPSIS
  Uji upgrade skema: bangun rilis lama dari git ref, migrasi + seed di database kosong, lalu terapkan migrasi
  versi kerja saat ini dan pastikan data lama tetap utuh. Database target wajib berakhiran _test.
.EXAMPLE
  powershell -File ops\windows\test-upgrade.ps1 -FromRef d9c3497 -DatabaseUrl 'postgres://pos:SANDI@localhost:5432/ats_upgrade_test?sslmode=disable'
#>
param([Parameter(Mandatory = $true)][string]$FromRef, [Parameter(Mandatory = $true)][string]$DatabaseUrl)
. "$PSScriptRoot\common.ps1"
$db = Get-DbParts $DatabaseUrl
if ($db.Db -notlike '*_test') { throw 'Database uji upgrade harus berakhiran _test.' }

$work = Join-Path $DataDir "upgrade-$FromRef"
if (Test-Path $work) { & git -C $Root worktree remove --force $work | Out-Null }
Write-Step "Worktree rilis lama $FromRef"
& git -C $Root worktree add --detach $work $FromRef | Out-Null
if ($LASTEXITCODE -ne 0) { throw "git worktree gagal untuk $FromRef" }

try {
    $env:PGPASSWORD = $db.Pass
    & dropdb -h $db.Host -p $db.Port -U $db.User --if-exists $db.Db
    & createdb -h $db.Host -p $db.Port -U $db.User -E UTF8 -T template0 $db.Db
    if ($LASTEXITCODE -ne 0) { throw 'createdb gagal' }

    Write-Step 'Build rilis lama'
    Invoke-Checked 'go' @('build', '-o', 'bin/', './cmd/...') (Join-Path $work 'backend')

    # Lingkungan sementara dengan kunci uji (bukan kunci instalasi).
    $env:APP_ENV = 'test'; $env:DATABASE_URL = $DatabaseUrl; $env:PUBLIC_ORIGIN = 'http://localhost:5173'; $env:ORG_SLUG = 'arunika'
    $env:SESSION_KEY = 'dXBncmFkZS1zZXNzaW9uLWtleS0wMDAwMDAwMDAwMDA='
    $env:NIK_ENC_KEY = 'dXBncmFkZS1uaWstZW5jLWtleS0wMDAwMDAwMDAwMDA='
    $env:NIK_INDEX_KEY = 'dXBncmFkZS1uaWstaWR4LWtleS0wMDAwMDAwMDAwMDA='
    $env:SEED_PASSWORD = 'Upgrade-Uji-2026'

    Write-Step 'Migrasi + seed rilis lama'
    Invoke-Checked (Join-Path $work "backend\bin\migrate$Exe") @('up')
    Invoke-Checked (Join-Path $work "backend\bin\seed$Exe") @()
    $q = "select (select count(*) from users)||','||(select count(*) from job_openings)||','||(select count(*) from candidate_identities)||','||(select count(*) from workflow_versions)"
    $before = (& psql -h $db.Host -p $db.Port -U $db.User -d $db.Db -tAc $q).Trim()
    $vOld = (& psql -h $db.Host -p $db.Port -U $db.User -d $db.Db -tAc 'select version from schema_migrations').Trim()

    Write-Step 'Migrasi versi kerja saat ini'
    Invoke-Checked 'go' @('build', '-o', 'bin/', './cmd/migrate') (Join-Path $Root 'backend')
    Invoke-Checked (Join-Path $Root "backend\bin\migrate$Exe") @('up')
    $after = (& psql -h $db.Host -p $db.Port -U $db.User -d $db.Db -tAc $q).Trim()
    $vNew = (& psql -h $db.Host -p $db.Port -U $db.User -d $db.Db -tAc 'select version from schema_migrations').Trim()
    if ($before -ne $after) { throw "Data berubah setelah upgrade: sebelum $before, sesudah $after" }
    Write-Ok "Upgrade skema $vOld -> $vNew, data utuh (users,jobs,identities,workflow_versions = $after)"
} finally {
    Remove-Item Env:\PGPASSWORD -ErrorAction SilentlyContinue
    & git -C $Root worktree remove --force $work | Out-Null
}
