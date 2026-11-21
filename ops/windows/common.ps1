# Fungsi bersama skrip Windows. Di-dot-source oleh skrip lain: . "$PSScriptRoot\common.ps1"
$ErrorActionPreference = 'Stop'
$script:Root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$script:EnvFile = Join-Path $script:Root '.env'
$script:DataDir = Join-Path $script:Root '.data'
$script:Exe = if ($IsLinux -or $IsMacOS) { '' } else { '.exe' }

function Write-Step([string]$msg) { Write-Host "==> $msg" -ForegroundColor Cyan }
function Write-Ok([string]$msg) { Write-Host "  [OK]    $msg" -ForegroundColor Green }
function Write-Fail([string]$msg) { Write-Host "  [GAGAL] $msg" -ForegroundColor Red }
function Write-Warn2([string]$msg) { Write-Host "  [PERINGATAN] $msg" -ForegroundColor Yellow }

# Membaca .env ke environment proses ini. Nilai berkutip dibuka; baris komentar dilewati. Nilai tidak dicetak.
function Import-DotEnv([string]$path = $script:EnvFile) {
    if (-not (Test-Path $path)) { throw ".env tidak ditemukan di $path. Jalankan ops\windows\new-env.ps1 terlebih dahulu." }
    foreach ($line in Get-Content -LiteralPath $path -Encoding UTF8) {
        $t = $line.Trim()
        if ($t -eq '' -or $t.StartsWith('#')) { continue }
        $i = $t.IndexOf('=')
        if ($i -lt 1) { continue }
        $k = $t.Substring(0, $i).Trim()
        $v = $t.Substring($i + 1).Trim()
        if ($v.Length -ge 2 -and (($v.StartsWith('"') -and $v.EndsWith('"')) -or ($v.StartsWith("'") -and $v.EndsWith("'")))) {
            $v = $v.Substring(1, $v.Length - 2)
        }
        [Environment]::SetEnvironmentVariable($k, $v, 'Process')
    }
}

# Mengurai DATABASE_URL postgres://user:pass@host:port/db?... menjadi bagian-bagiannya.
function Get-DbParts([string]$url) {
    $m = [regex]::Match($url, '^postgres(?:ql)?://(?<user>[^:@/]+)(?::(?<pass>[^@]*))?@(?<host>[^:/?]+)(?::(?<port>\d+))?/(?<db>[^?]+)')
    if (-not $m.Success) { throw "DATABASE_URL tidak dikenali. Format: postgres://user:sandi@host:5432/nama_db?sslmode=disable" }
    $port = if ($m.Groups['port'].Success) { $m.Groups['port'].Value } else { '5432' }
    return @{ User = $m.Groups['user'].Value; Pass = [uri]::UnescapeDataString($m.Groups['pass'].Value); Host = $m.Groups['host'].Value; Port = $port; Db = $m.Groups['db'].Value }
}

function Test-Command([string]$name) { return [bool](Get-Command $name -ErrorAction SilentlyContinue) }

function Test-TcpPort([string]$hostName, [int]$port, [int]$timeoutMs = 1500) {
    $c = New-Object System.Net.Sockets.TcpClient
    try {
        $ar = $c.BeginConnect($hostName, $port, $null, $null)
        if (-not $ar.AsyncWaitHandle.WaitOne($timeoutMs)) { return $false }
        $c.EndConnect($ar)
        return $true
    } catch { return $false } finally { $c.Close() }
}

function Invoke-Checked([string]$file, [string[]]$arguments, [string]$workDir = $script:Root) {
    # PowerShell 5.1 menolak -ArgumentList kosong, jadi parameter hanya dikirim bila ada.
    $sp = @{ FilePath = $file; WorkingDirectory = $workDir; NoNewWindow = $true; Wait = $true; PassThru = $true }
    if ($arguments -and $arguments.Count -gt 0) { $sp.ArgumentList = $arguments }
    $p = Start-Process @sp
    if ($p.ExitCode -ne 0) { throw "Perintah gagal ($($p.ExitCode)): $file $($arguments -join ' ')" }
}
