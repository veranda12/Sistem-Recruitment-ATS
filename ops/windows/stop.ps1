<#
.SYNOPSIS
  Menghentikan proses yang dijalankan start.ps1 (berdasarkan .data\pids.json). Tidak menyentuh proses lain.
#>
. "$PSScriptRoot\common.ps1"
$pidFile = Join-Path $DataDir 'pids.json'
if (-not (Test-Path $pidFile)) { Write-Ok 'Tidak ada proses tercatat.'; exit 0 }
$ids = Get-Content -LiteralPath $pidFile -Raw | ConvertFrom-Json
foreach ($p in $ids.PSObject.Properties) {
    $proc = Get-Process -Id $p.Value -ErrorAction SilentlyContinue
    if ($proc) {
        # npm menjalankan node sebagai anak proses; hentikan seluruh pohon proses milik PID tersebut.
        & taskkill.exe /PID $p.Value /T /F | Out-Null
        Write-Ok "$($p.Name) (PID $($p.Value)) dihentikan"
    } else { Write-Ok "$($p.Name) sudah tidak berjalan" }
}
Remove-Item -LiteralPath $pidFile
