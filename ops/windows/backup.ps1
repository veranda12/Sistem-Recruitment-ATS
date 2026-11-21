<#
.SYNOPSIS
  Backup logis PostgreSQL (pg_dump -Fc) + salinan object storage berkas + manifest SHA-256.
  Kunci enkripsi (NIK_ENC_KEY, NIK_INDEX_KEY, SESSION_KEY) TIDAK ikut; simpan terpisah di brankas.
.EXAMPLE
  powershell -File ops\windows\backup.ps1 -DatabaseUrl $env:DATABASE_URL -ObjectDir .data\objects -OutDir D:\backup\ats
#>
param([Parameter(Mandatory = $true)][string]$DatabaseUrl, [Parameter(Mandatory = $true)][string]$ObjectDir, [Parameter(Mandatory = $true)][string]$OutDir)
. "$PSScriptRoot\common.ps1"
$db = Get-DbParts $DatabaseUrl
$stamp = (Get-Date).ToString('yyyyMMdd-HHmmss')
$dest = Join-Path $OutDir "ats-$($db.Db)-$stamp"
New-Item -ItemType Directory -Force $dest | Out-Null
$dest = (Resolve-Path $dest).Path
$sw = [Diagnostics.Stopwatch]::StartNew()

Write-Step "pg_dump $($db.Db)"
$env:PGPASSWORD = $db.Pass
try {
    & pg_dump -h $db.Host -p $db.Port -U $db.User -Fc -Z 6 --no-owner -f (Join-Path $dest 'db.dump') $db.Db
    if ($LASTEXITCODE -ne 0) { throw 'pg_dump gagal' }
    $lsn = (& psql -h $db.Host -p $db.Port -U $db.User -d $db.Db -tAc 'select pg_current_wal_lsn()').Trim()
} finally { Remove-Item Env:\PGPASSWORD -ErrorAction SilentlyContinue }

Write-Step 'Salin object storage'
$objDest = Join-Path $dest 'objects'
if (Test-Path $ObjectDir) { Copy-Item -Recurse -Force $ObjectDir $objDest } else { New-Item -ItemType Directory $objDest | Out-Null }

Write-Step 'Manifest'
$files = Get-ChildItem -Recurse -File $dest | ForEach-Object {
    [pscustomobject]@{ path = $_.FullName.Substring($dest.Length + 1).Replace('\', '/'); bytes = $_.Length; sha256 = (Get-FileHash -Algorithm SHA256 $_.FullName).Hash.ToLower() }
}
$manifest = [pscustomobject]@{ database = $db.Db; createdAt = (Get-Date).ToUniversalTime().ToString('o'); walLsnAtEnd = $lsn; files = $files; durationSeconds = [math]::Round($sw.Elapsed.TotalSeconds, 1) }
$manifest | ConvertTo-Json -Depth 4 | Set-Content -Encoding UTF8 (Join-Path $dest 'manifest.json')
Write-Ok "Backup selesai di $dest ($($files.Count) berkas, $([math]::Round($sw.Elapsed.TotalSeconds,1)) s)"
Write-Output $dest
