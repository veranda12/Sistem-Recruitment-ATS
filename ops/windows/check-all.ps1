<#
.SYNOPSIS
  Menjalankan seluruh pemeriksaan: gofmt, go vet, drift sqlc, go test (DB riil), typecheck, lint, Vitest, build, Playwright.
.PARAMETER TestDatabaseUrl
  Database khusus test (akan DIHAPUS isinya). Nama wajib berakhiran _test.
.EXAMPLE
  powershell -File ops\windows\check-all.ps1 -TestDatabaseUrl 'postgres://pos:SANDI@localhost:5432/ats_test?sslmode=disable'
#>
param([Parameter(Mandatory = $true)][string]$TestDatabaseUrl, [switch]$SkipE2E)
. "$PSScriptRoot\common.ps1"
if (-not ((Get-DbParts $TestDatabaseUrl).Db -like '*_test')) { throw 'Nama database test harus berakhiran _test (isinya dihapus saat test).' }
$backend = Join-Path $Root 'backend'
$frontend = Join-Path $Root 'frontend'
$results = [ordered]@{}
function Run([string]$name, [string]$file, [string[]]$a, [string]$dir) {
    Write-Step $name
    $p = Start-Process -FilePath $file -ArgumentList $a -WorkingDirectory $dir -NoNewWindow -Wait -PassThru
    $script:results[$name] = if ($p.ExitCode -eq 0) { 'lulus' } else { "GAGAL ($($p.ExitCode))" }
}

Write-Step 'gofmt'
$unformatted = & gofmt -l $backend
$results['gofmt'] = if ($unformatted) { "GAGAL: $unformatted" } else { 'lulus' }
Run 'go vet' 'go' @('vet', './...') $backend
if (Test-Command 'sqlc') { Run 'sqlc diff (drift kode hasil generate)' 'sqlc' @('diff') $backend } else { $results['sqlc diff'] = 'dilewati (sqlc tidak ada)' }
$env:DATABASE_URL_TEST = $TestDatabaseUrl
Run 'go test (PostgreSQL riil)' 'go' @('test', './...', '-count=1', '-p', '1') $backend
Run 'build backend' 'go' @('build', '-o', 'bin/', './cmd/...') $backend
Run 'typecheck frontend + kontrak OpenAPI' 'npm.cmd' @('run', 'gen:api') $frontend
Run 'typecheck' 'npm.cmd' @('run', 'typecheck') $frontend
Run 'lint' 'npm.cmd' @('run', 'lint') $frontend
Run 'vitest' 'npm.cmd' @('test') $frontend
Run 'build frontend' 'npm.cmd' @('run', 'build') $frontend
if (-not $SkipE2E) { Run 'playwright' 'npx.cmd' @('playwright', 'test') $frontend }

Write-Host ''
Write-Host 'Ringkasan:'
$bad = 0
foreach ($k in $results.Keys) {
    $v = $results[$k]
    if ($v -like 'GAGAL*') { $bad++; Write-Fail "$k : $v" } else { Write-Ok "$k : $v" }
}
if ($bad -gt 0) { exit 1 }
