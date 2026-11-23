<#
.SYNOPSIS
  Mencari alat bantu pembaca CV dan pemindai (Poppler, Tesseract + data bahasa, LibreOffice, ClamAV) di PATH dan
  lokasi pemasangan umum, lalu mengisi variabel CV_* yang masih kosong di .env. Nilai yang sudah diisi tidak diubah
  kecuali dengan -Force. Dipanggil otomatis oleh new-env.ps1 dan setup.ps1; aman dijalankan berulang.
.PARAMETER Install
  Pasang lewat winget alat yang belum ditemukan: Poppler (portabel) dan Tesseract, lalu unduh data bahasa
  ind/eng/osd (cv-tools.ps1). LibreOffice hanya dengan -WithLibreOffice (~350 MB).
.PARAMETER NoDownload
  Jangan mengunduh data bahasa Tesseract (ind/eng/osd) walau belum ada. Tanpa ini, data bahasa diunduh otomatis
  ke tools	essdata bila Tesseract terpasang.
.PARAMETER Force
  Timpa nilai CV_* yang sudah ada dengan hasil deteksi.
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File ops\windows\detect-tools.ps1
  powershell -ExecutionPolicy Bypass -File ops\windows\detect-tools.ps1 -Install
#>
param([switch]$Install, [switch]$WithLibreOffice, [switch]$Force, [switch]$NoDownload)
. "$PSScriptRoot\common.ps1"
if (-not (Test-Path $EnvFile)) { throw ".env belum ada. Jalankan new-env.ps1 terlebih dahulu." }

$pf = $env:ProgramFiles
$pf86 = ${env:ProgramFiles(x86)}
$local = $env:LOCALAPPDATA

# Mengembalikan path pertama yang ada: dari PATH (Get-Command) lalu pola lokasi umum (wildcard diizinkan).
function Find-Tool([string]$command, [string[]]$patterns) {
    $c = Get-Command $command -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($c -and $c.Source -and (Test-Path $c.Source)) { return $c.Source }
    foreach ($p in $patterns) {
        if (-not $p) { continue }
        $hit = Get-ChildItem -Path $p -ErrorAction SilentlyContinue | Sort-Object FullName -Descending | Select-Object -First 1
        if ($hit) { return $hit.FullName }
    }
    return $null
}

function Find-Poppler {
    $pdftoppm = Find-Tool 'pdftoppm' @(
        'C:\poppler*\Library\bin\pdftoppm.exe', 'C:\poppler*\bin\pdftoppm.exe',
        'D:\Tools\poppler*\Library\bin\pdftoppm.exe', 'D:\poppler*\Library\bin\pdftoppm.exe',
        "$pf\poppler*\Library\bin\pdftoppm.exe", "$pf\poppler*\bin\pdftoppm.exe",
        "$local\Microsoft\WinGet\Packages\oschwartz10612.Poppler*\poppler-*\Library\bin\pdftoppm.exe")
    # Alias winget (WinGet\Links) menunjuk ke berkas asli; pakai folder asli agar pdfinfo dkk. ditemukan bersama.
    if ($pdftoppm) {
        $item = Get-Item -LiteralPath $pdftoppm
        if ($item.LinkType -and $item.Target) { $pdftoppm = @($item.Target)[0] }
        return Split-Path -Parent $pdftoppm
    }
    return $null
}

function Find-Tesseract {
    Find-Tool 'tesseract' @("$pf\Tesseract-OCR\tesseract.exe", "$pf86\Tesseract-OCR\tesseract.exe",
        "$local\Programs\Tesseract-OCR\tesseract.exe", 'D:\Tools\Tesseract-OCR\tesseract.exe')
}

# Data bahasa wajib ind, eng, osd. Utamakan tools\tessdata (versi terkunci dari cv-tools.ps1).
function Find-TessData([string]$tesseract) {
    $cands = @((Join-Path $Root 'tools\tessdata'))
    if ($tesseract) { $cands += Join-Path (Split-Path -Parent $tesseract) 'tessdata' }
    foreach ($d in $cands) {
        if (@('ind', 'eng', 'osd' | Where-Object { Test-Path (Join-Path $d "$_.traineddata") }).Count -eq 3) { return $d }
    }
    return $null
}

function Find-Soffice {
    Find-Tool 'soffice.com' @("$pf\LibreOffice\program\soffice.com", "$pf86\LibreOffice\program\soffice.com",
        'D:\Tools\LibreOffice\program\soffice.com')
}

function Find-ClamdScan {
    Find-Tool 'clamdscan' @('D:\Tools\ClamAV\clamdscan.exe', "$pf\ClamAV\clamdscan.exe", 'C:\Tools\ClamAV\clamdscan.exe')
}

function Install-Winget([string]$id, [string]$label) {
    if (-not (Test-Command 'winget')) { Write-Warn2 "winget tidak ada; pasang $label manual."; return }
    Write-Step "Memasang $label ($id) lewat winget"
    & winget install --id $id --exact --accept-package-agreements --accept-source-agreements --disable-interactivity
    if ($LASTEXITCODE -ne 0) { Write-Warn2 "winget gagal memasang $label (kode $LASTEXITCODE)." }
    # PATH proses ini tidak diperbarui winget; folder Links dan lokasi umum dicari ulang oleh Find-*.
    $env:PATH = "$env:PATH;$local\Microsoft\WinGet\Links"
}

Write-Step 'Mencari alat bantu pembaca CV'
$popplerDir = Find-Poppler
$tesseract = Find-Tesseract
if ($Install) {
    if (-not $popplerDir) { Install-Winget 'oschwartz10612.Poppler' 'Poppler'; $popplerDir = Find-Poppler }
    if (-not $tesseract) { Install-Winget 'UB-Mannheim.TesseractOCR' 'Tesseract'; $tesseract = Find-Tesseract }
    if ($WithLibreOffice -and -not (Find-Soffice)) { Install-Winget 'TheDocumentFoundation.LibreOffice' 'LibreOffice' }
}
$tessdata = Find-TessData $tesseract
# Data bahasa kecil (beberapa MB, versi dan SHA-256 dikunci) sehingga diunduh otomatis bila Tesseract ada.
if (-not $tessdata -and ($tesseract -or $Install) -and -not $NoDownload) {
    try {
        & "$PSScriptRoot\cv-tools.ps1" -SkipRedis
        $tessdata = Find-TessData $tesseract
    } catch { Write-Warn2 "Unduh data bahasa gagal: $($_.Exception.Message)" }
}
$soffice = Find-Soffice
$clamd = Find-ClamdScan

# Path ditulis dengan garis miring agar sama di semua alat; dikutip bila berisi spasi.
function Fmt([string]$p) { if (-not $p) { return '' }; $p = $p -replace '\\', '/'; if ($p -match ' ') { return "`"$p`"" }; return $p }
function PopplerExe([string]$name) { if ($popplerDir) { return Fmt (Join-Path $popplerDir $name) }; return '' }
$found = [ordered]@{
    CV_PDFINFO   = (PopplerExe 'pdfinfo.exe')
    CV_PDFTOTEXT = (PopplerExe 'pdftotext.exe')
    CV_PDFTOPPM  = (PopplerExe 'pdftoppm.exe')
    CV_PDFIMAGES = (PopplerExe 'pdfimages.exe')
    CV_TESSERACT = (Fmt $tesseract)
    CV_TESSDATA  = (Fmt $tessdata)
    CV_SOFFICE   = (Fmt $soffice)
    CV_WORK_DIR  = (Fmt (Join-Path $DataDir 'cvwork'))
}

$lines = Get-Content -LiteralPath $EnvFile -Encoding UTF8
$seen = @{}
$changed = 0
$out = @(foreach ($l in $lines) {
    $m = [regex]::Match($l, '^(?<k>CV_[A-Z_]+)=(?<v>.*)$')
    if ($m.Success -and $found.Contains($m.Groups['k'].Value)) {
        $k = $m.Groups['k'].Value
        $seen[$k] = $true
        $cur = $m.Groups['v'].Value.Trim()
        if ($found[$k] -and ($Force -or $cur -eq '')) {
            if ($cur -ne $found[$k]) { $changed++ }
            "$k=$($found[$k])"
            continue
        }
    }
    $l
})
# .env lama mungkin belum punya kunci tertentu: tambahkan di akhir.
foreach ($k in $found.Keys) {
    if (-not $seen[$k] -and $found[$k]) { $out += "$k=$($found[$k])"; $changed++ }
}
[System.IO.File]::WriteAllLines($EnvFile, [string[]]$out, (New-Object System.Text.UTF8Encoding($false)))

Write-Step 'Hasil'
$report = @(
    @('Poppler (PDF)', $popplerDir, 'wajib untuk PDF', 'winget install oschwartz10612.Poppler'),
    @('Tesseract (OCR)', $tesseract, 'wajib untuk scan/foto', 'winget install UB-Mannheim.TesseractOCR'),
    @('Data bahasa ind/eng/osd', $tessdata, 'wajib untuk OCR', 'powershell -File ops\windows\cv-tools.ps1 -SkipRedis'),
    @('LibreOffice', $soffice, 'opsional: .doc lawas, DOCX bergambar', 'winget install TheDocumentFoundation.LibreOffice')
)
foreach ($r in $report) {
    if ($r[1]) { Write-Ok "$($r[0]): $($r[1])" } else { Write-Warn2 "$($r[0]) tidak ditemukan ($($r[2])). Pasang: $($r[3]), atau jalankan detect-tools.ps1 -Install" }
}
if ($clamd) {
    Write-Ok "ClamAV ditemukan: $clamd. Untuk pemindai sungguhan lihat bagian Pemindai di docs\operations\windows-dev.md (SCAN_COMMAND tidak diubah otomatis)."
}
Write-Ok "$changed nilai di .env diisi/diperbarui. Nilai yang sudah ada dipertahankan$(if ($Force) { ' (kecuali -Force)' })."
if (-not $popplerDir -or -not $tesseract -or -not $tessdata) {
    Write-Warn2 'Tanpa alat wajib di atas, aplikasi tetap berjalan; unggahan CV yang membutuhkan alat itu gagal dibaca dan kandidat diminta mengisi profil manual.'
}
