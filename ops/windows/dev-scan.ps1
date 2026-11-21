<#
.SYNOPSIS
  PEMINDAI KHUSUS PENGEMBANGAN. Hanya mengenali string uji EICAR. BUKAN pemindai malware.
  Produksi wajib memakai pemindai sungguhan, mis. SCAN_COMMAND="clamdscan --no-summary --fdpass".
  Kontrak: exit 0 = bersih, exit 1 = terinfeksi, lainnya = gagal (dokumen tetap pending dan dicoba ulang).
#>
param([Parameter(Mandatory = $true)][string]$Path)
if (-not (Test-Path -LiteralPath $Path)) { exit 2 }
$text = [System.Text.Encoding]::ASCII.GetString([System.IO.File]::ReadAllBytes($Path))
if ($text.Contains('EICAR-STANDARD-ANTIVIRUS-TEST-FILE')) { exit 1 }
exit 0
