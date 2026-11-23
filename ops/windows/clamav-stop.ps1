<#
.SYNOPSIS
  Menghentikan clamd yang dijalankan clamav-start.ps1 (berdasarkan .data\clamd.pid). Selama clamd mati, dokumen
  baru tetap "pending" dan dipindai ulang oleh worker setelah clamd kembali.
#>
. "$PSScriptRoot\common.ps1"
$pidFile = Join-Path $DataDir 'clamd.pid'
if (-not (Test-Path $pidFile)) { Write-Ok 'clamd tidak tercatat berjalan.'; exit 0 }
$id = [int](Get-Content -LiteralPath $pidFile -Raw)
$proc = Get-Process -Id $id -ErrorAction SilentlyContinue
if ($proc -and $proc.ProcessName -like 'clamd*') {
    & taskkill.exe /PID $id /T /F | Out-Null
    Write-Ok "clamd (PID $id) dihentikan"
} else { Write-Ok 'clamd sudah tidak berjalan' }
Remove-Item -LiteralPath $pidFile
