<#
.SYNOPSIS
  Menjalankan clamd (ClamAV) lokal di 127.0.0.1:3310 untuk memindai dokumen unggahan (SCAN_COMMAND).
  Hanya berjalan saat skrip ini dipanggil: tidak dipasang sebagai service. Hentikan dengan clamav-stop.ps1.
  ClamAV dipasang terpisah (zip resmi Cisco-Talos) di -ClamDir, dengan clamd.conf dan freshclam.conf di sana.
.PARAMETER ClamDir
  Folder ClamAV (bawaan $env:CLAMAV_DIR atau D:\Tools\ClamAV).
.PARAMETER Update
  Perbarui signature dengan freshclam sebelum menjalankan clamd.
.EXAMPLE
  powershell -File ops\windows\clamav-start.ps1 -Update
#>
param([string]$ClamDir = $(if ($env:CLAMAV_DIR) { $env:CLAMAV_DIR } else { 'D:\Tools\ClamAV' }), [switch]$Update)
. "$PSScriptRoot\common.ps1"

$clamd = Join-Path $ClamDir 'clamd.exe'
if (-not (Test-Path $clamd)) { throw "clamd.exe tidak ditemukan di $ClamDir. Pasang ClamAV dan buat clamd.conf (lihat docs\operations\windows-dev.md)." }
if ($Update) {
    & (Join-Path $ClamDir 'freshclam.exe') "--config-file=$(Join-Path $ClamDir 'freshclam.conf')"
    if ($LASTEXITCODE -ne 0) { Write-Warning "freshclam exit $LASTEXITCODE; memakai signature yang ada" }
}
if (-not (Get-ChildItem (Join-Path $ClamDir 'db') -Filter '*.c?d' -ErrorAction SilentlyContinue)) { throw "Database signature kosong. Jalankan dengan -Update." }
if (Test-TcpPort '127.0.0.1' 3310) { Write-Ok 'clamd sudah berjalan di 127.0.0.1:3310'; exit 0 }

$logs = Join-Path $DataDir 'logs'
New-Item -ItemType Directory -Force $logs | Out-Null
$p = Start-Process -FilePath $clamd -ArgumentList "--config-file=$(Join-Path $ClamDir 'clamd.conf')" -WindowStyle Hidden -PassThru `
    -RedirectStandardOutput (Join-Path $logs 'clamd.log') -RedirectStandardError (Join-Path $logs 'clamd.err.log')
$p.Id | Set-Content -LiteralPath (Join-Path $DataDir 'clamd.pid')
# Memuat signature butuh ~20-60 detik.
$deadline = (Get-Date).AddSeconds(120)
while (-not (Test-TcpPort '127.0.0.1' 3310) -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 500 }
if (-not (Test-TcpPort '127.0.0.1' 3310)) { throw "clamd tidak siap. Lihat $logs\clamd.err.log" }
Write-Ok "clamd berjalan (PID $($p.Id)). Isi SCAN_COMMAND=$(Join-Path $ClamDir 'clamdscan.exe') --no-summary --config-file=$(Join-Path $ClamDir 'clamd.conf') di .env lalu restart start.ps1."
