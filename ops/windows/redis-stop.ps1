<#
.SYNOPSIS
  Menghentikan Redis yang dijalankan redis-start.ps1 (berdasarkan .data\redis.pid). Antrean CV tetap aman:
  PostgreSQL adalah sumber kebenaran dan worker CV memproses langsung dari PostgreSQL saat Redis mati.
#>
. "$PSScriptRoot\common.ps1"
$pidFile = Join-Path $DataDir 'redis.pid'
if (-not (Test-Path $pidFile)) { Write-Ok 'Redis tidak tercatat berjalan.'; exit 0 }
$id = [int](Get-Content -LiteralPath $pidFile -Raw)
$proc = Get-Process -Id $id -ErrorAction SilentlyContinue
if ($proc -and $proc.ProcessName -like 'redis-server*') {
    & taskkill.exe /PID $id /T /F | Out-Null
    Write-Ok "Redis (PID $id) dihentikan"
} else { Write-Ok 'Redis sudah tidak berjalan' }
Remove-Item -LiteralPath $pidFile
