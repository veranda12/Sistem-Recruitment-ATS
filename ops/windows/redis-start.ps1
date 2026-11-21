<#
.SYNOPSIS
  Menjalankan Redis lokal (127.0.0.1:6379) di latar belakang untuk antrean CV (Asynq) dan cache.
  Hanya berjalan saat skrip ini dipanggil: tidak dipasang sebagai service, tidak berjalan saat komputer menyala.
  Data di .data\redis, log di .data\logs\redis.log. Hentikan dengan redis-stop.ps1.
.EXAMPLE
  powershell -File ops\windows\redis-start.ps1
#>
. "$PSScriptRoot\common.ps1"

$server = Join-Path $Root 'tools\redis\Redis-8.10.3-Windows-x64-msys2\redis-server.exe'
if (-not (Test-Path $server)) { throw "Redis belum dipasang. Jalankan ops\windows\cv-tools.ps1 terlebih dahulu." }
if (Test-TcpPort '127.0.0.1' 6379) { Write-Ok 'Redis sudah berjalan di 127.0.0.1:6379'; exit 0 }

$dir = Join-Path $DataDir 'redis'
$logs = Join-Path $DataDir 'logs'
New-Item -ItemType Directory -Force $dir, $logs | Out-Null
# Konfigurasi minimal dev: hanya loopback, protected mode, AOF untuk ketahanan antrean saat restart.
$conf = Join-Path $dir 'redis.conf'
@"
bind 127.0.0.1
port 6379
protected-mode yes
dir ./
appendonly yes
appendfsync everysec
save 300 10
maxmemory 256mb
maxmemory-policy noeviction
"@ | Set-Content -LiteralPath $conf -Encoding ASCII

$p = Start-Process -FilePath $server -ArgumentList 'redis.conf' -WorkingDirectory $dir -WindowStyle Hidden -PassThru `
    -RedirectStandardOutput (Join-Path $logs 'redis.log') -RedirectStandardError (Join-Path $logs 'redis.err.log')
$p.Id | Set-Content -LiteralPath (Join-Path $DataDir 'redis.pid')
$deadline = (Get-Date).AddSeconds(10)
while (-not (Test-TcpPort '127.0.0.1' 6379) -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 300 }
if (-not (Test-TcpPort '127.0.0.1' 6379)) { throw "Redis tidak siap. Lihat $logs\redis.log" }
Write-Ok "Redis berjalan (PID $($p.Id)) di 127.0.0.1:6379. Isi REDIS_URL=redis://127.0.0.1:6379/0 di .env lalu restart start.ps1."
