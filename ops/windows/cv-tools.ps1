<#
.SYNOPSIS
  Mengunduh komponen pembaca CV yang versinya dikunci ke folder tools\ (tidak masuk Git), dengan verifikasi SHA-256:
  data bahasa Tesseract (ind, eng, osd; tessdata_fast commit 87416418) dan Redis untuk Windows (8.10.3, msys2,
  tanpa service sehingga tidak berjalan otomatis saat komputer menyala).
  Tesseract, Poppler, dan LibreOffice dipasang terpisah (lihat docs\cv-parsing.md).
.EXAMPLE
  powershell -File ops\windows\cv-tools.ps1
#>
param([switch]$SkipRedis)
. "$PSScriptRoot\common.ps1"

$tools = Join-Path $Root 'tools'
New-Item -ItemType Directory -Force (Join-Path $tools 'tessdata') | Out-Null

function Get-Pinned([string]$url, [string]$dest, [string]$sha256) {
    if ((Test-Path $dest) -and ((Get-FileHash -Algorithm SHA256 -LiteralPath $dest).Hash -eq $sha256.ToUpper())) {
        Write-Ok "$(Split-Path $dest -Leaf) sudah ada dan cocok"
        return
    }
    $tmp = "$dest.part"
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $tmp
    $h = (Get-FileHash -Algorithm SHA256 -LiteralPath $tmp).Hash
    if ($h -ne $sha256.ToUpper()) {
        Remove-Item -LiteralPath $tmp
        throw "Checksum $(Split-Path $dest -Leaf) tidak cocok: $h"
    }
    Move-Item -Force -LiteralPath $tmp -Destination $dest
    Write-Ok "$(Split-Path $dest -Leaf) diunduh dan terverifikasi"
}

Write-Step 'Data bahasa Tesseract (tessdata_fast, Apache-2.0)'
$commit = '87416418657359cb625c412a48b6e1d6d41c29bd'
$langs = @{
    ind = '69786901da87ab8766c1ea7fbb10b28f2110c14da3f6c8f2735df131fba95d88'
    eng = '7d4322bd2a7749724879683fc3912cb542f19906c83bcc1a52132556427170b2'
    osd = '9cf5d576fcc47564f11265841e5ca839001e7e6f38ff7f7aacf46d15a96b00ff'
}
foreach ($l in $langs.Keys) {
    Get-Pinned "https://raw.githubusercontent.com/tesseract-ocr/tessdata_fast/$commit/$l.traineddata" (Join-Path $tools "tessdata\$l.traineddata") $langs[$l]
}

if (-not $SkipRedis) {
    Write-Step 'Redis untuk Windows (redis-windows/redis-windows 8.10.3, msys2, tanpa service)'
    $zip = Join-Path $tools 'redis-8.10.3.zip'
    Get-Pinned 'https://github.com/redis-windows/redis-windows/releases/download/8.10.3/Redis-8.10.3-Windows-x64-msys2.zip' $zip 'af16df340525b431e9ec8dabfd625d5e7a233f82805b5418819904ea909c447f'
    $dst = Join-Path $tools 'redis'
    if (-not (Test-Path (Join-Path $dst 'Redis-8.10.3-Windows-x64-msys2\redis-server.exe'))) {
        Expand-Archive -Force -LiteralPath $zip -DestinationPath $dst
    }
    Write-Ok 'Redis siap. Jalankan manual: ops\windows\redis-start.ps1 (tidak berjalan otomatis saat boot)'
}
