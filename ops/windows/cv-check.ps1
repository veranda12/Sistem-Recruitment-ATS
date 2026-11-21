<#
.SYNOPSIS
  Pemeriksaan pembaca CV: preflight (fixture kecil sungguhan), pembuatan fixture sintetis, test Go (unit + kualitas
  + integrasi PostgreSQL), dan evaluasi/benchmark dengan laporan JSON.
.PARAMETER Mode
  preflight | fixtures | test | eval | all
.EXAMPLE
  powershell -File ops\windows\cv-check.ps1 -Mode all
#>
param([ValidateSet('preflight', 'fixtures', 'test', 'eval', 'all')][string]$Mode = 'all')
. "$PSScriptRoot\common.ps1"
Import-DotEnv
$backend = Join-Path $Root 'backend'
$fixtures = Join-Path $Root 'tests\cv-fixtures\out'
$env:CV_FIXTURES = $fixtures

if ($Mode -in 'preflight', 'all') {
    Write-Step 'Preflight pembaca CV'
    Invoke-Checked 'go' @('run', './cmd/cvworker', 'preflight') $backend
}
if ($Mode -in 'fixtures', 'all') {
    Write-Step 'Fixture CV sintetis + golden'
    if (-not $env:CV_PDFTOPPM) { throw 'CV_PDFTOPPM belum diisi di .env' }
    if (-not $env:CV_PYTHON) { $py = Get-Command python -ErrorAction SilentlyContinue; if ($py) { $env:CV_PYTHON = $py.Source } }
    Invoke-Checked 'go' @('run', './cmd/cvfixtures', '-out', $fixtures) $backend
}
if ($Mode -in 'test', 'all') {
    Write-Step 'Test Go pembaca CV (unit, kualitas fixture, integrasi)'
    if (-not $env:DATABASE_URL_TEST) { $env:DATABASE_URL_TEST = $env:DATABASE_URL -replace '/ats_dev', '/ats_test' }
    Invoke-Checked 'go' @('test', '-count=1', '-timeout', '30m', './internal/cvparse/...', './internal/cvreview/...', './internal/app/', '-run', 'CVParse|Fixture|Parse|Normalize|Docx|CFB|Detect|TwoColumn|Sensitive|Evidence') $backend
}
if ($Mode -in 'eval', 'all') {
    Write-Step 'Evaluasi kualitas + waktu proses'
    $report = Join-Path $Root 'docs\cv-parsing-eval.json'
    Invoke-Checked 'go' @('run', './cmd/cveval', '-fixtures', $fixtures, '-report', $report) $backend
    Write-Ok "Laporan: $report"
}
