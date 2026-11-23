<#
.SYNOPSIS
  Membuat .env dari .env.example dengan kunci acak baru (32 byte, base64). Tidak menimpa .env yang sudah ada kecuali -Force.
.EXAMPLE
  powershell -File ops\windows\new-env.ps1 -DatabaseUrl 'postgres://pos:SANDI@localhost:5432/ats_dev?sslmode=disable' -SeedPassword 'Dev-Sandi-2026'
.PARAMETER ApiPort
  Port API lokal (bawaan 8080). Ganti bila 8080 dipakai aplikasi lain; proxy frontend dev mengikuti otomatis.
#>
param(
    [Parameter(Mandatory = $true)][string]$DatabaseUrl,
    [string]$SeedPassword = '',
    [string]$RedisUrl = '',
    [ValidateRange(1024, 65535)][int]$ApiPort = 8080,
    [switch]$Force
)
. "$PSScriptRoot\common.ps1"

if ((Test-Path $EnvFile) -and -not $Force) { throw ".env sudah ada. Pakai -Force untuk membuat ulang (kunci lama hilang: NIK terenkripsi tidak bisa dibuka lagi)." }

function New-Key {
    $b = New-Object byte[] 32
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b)
    return [Convert]::ToBase64String($b)
}

$lines = Get-Content -LiteralPath (Join-Path $Root '.env.example') -Encoding UTF8
$out = foreach ($l in $lines) {
    switch -regex ($l) {
        '^DATABASE_URL=' { "DATABASE_URL=$DatabaseUrl"; break }
        '^HTTP_ADDR=' { "HTTP_ADDR=127.0.0.1:$ApiPort"; break }
        '^SESSION_KEY=' { "SESSION_KEY=$(New-Key)"; break }
        '^NIK_ENC_KEY=' { "NIK_ENC_KEY=$(New-Key)"; break }
        '^NIK_INDEX_KEY=' { "NIK_INDEX_KEY=$(New-Key)"; break }
        '^SEED_PASSWORD=' { "SEED_PASSWORD=$SeedPassword"; break }
        '^REDIS_URL=' { "REDIS_URL=$RedisUrl"; break }
        '^MAIL_DIR=' { "MAIL_DIR=$(Join-Path $DataDir 'mail')"; break }
        '^OBJECT_DIR=' { "OBJECT_DIR=$(Join-Path $DataDir 'objects')"; break }
        # Pemindai khusus dev (hanya mengenali berkas uji EICAR); tanpa ini CV tertahan "Menunggu pemeriksaan file".
        '^SCAN_COMMAND=' { "SCAN_COMMAND=`"powershell -NoProfile -ExecutionPolicy Bypass -File $(Join-Path $PSScriptRoot 'dev-scan.ps1')`""; break }
        default { $l }
    }
}
# UTF-8 tanpa BOM agar terbaca juga oleh alat lain.
[System.IO.File]::WriteAllLines($EnvFile, $out, (New-Object System.Text.UTF8Encoding($false)))
Write-Ok ".env dibuat di $EnvFile. Simpan salinan NIK_ENC_KEY dan NIK_INDEX_KEY di brankas terpisah dari backup database."
