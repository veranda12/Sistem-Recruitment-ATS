<#
.SYNOPSIS
  Uji asap terhadap instance yang berjalan: readiness, lowongan publik, login staf seed, daftar lamaran, logout.
.EXAMPLE
  powershell -File ops\windows\smoke.ps1 -BaseUrl http://127.0.0.1:8080 -Origin http://localhost:5173
#>
param([string]$BaseUrl = 'http://127.0.0.1:8080', [string]$Origin = '', [string]$Email = 'hr.ops@arunika.example', [string]$Password = '')
. "$PSScriptRoot\common.ps1"
if (Test-Path $EnvFile) { Import-DotEnv }
if (-not $Origin) { $Origin = $env:PUBLIC_ORIGIN }
if (-not $Password) { $Password = $env:SEED_PASSWORD }
$failed = 0
function Check([string]$name, [scriptblock]$body) {
    try { $msg = & $body; Write-Ok "$name $msg" } catch { Write-Fail "$name : $($_.Exception.Message)"; $script:failed++ }
}

Check 'readyz' { $r = Invoke-RestMethod "$BaseUrl/readyz" -TimeoutSec 5; if ($r.dependencies.database -ne 'ok') { throw 'database tidak ok' }; "(redis: $($r.dependencies.redis))" }
Check 'lowongan publik' { $r = Invoke-RestMethod "$BaseUrl/api/v1/jobs" -TimeoutSec 5; "($($r.items.Count) lowongan)" }
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$csrf = ''
Check "login $Email" {
    $body = @{ email = $Email; password = $Password } | ConvertTo-Json
    $r = Invoke-RestMethod "$BaseUrl/api/v1/auth/login" -Method Post -Body $body -ContentType 'application/json' -Headers @{ Origin = $Origin } -WebSession $session
    $script:csrf = $r.csrfToken
    "(peran: $($r.roles -join ', '))"
}
Check 'daftar lamaran terscope' { $r = Invoke-RestMethod "$BaseUrl/api/v1/applications" -WebSession $session; "($($r.items.Count) lamaran)" }
Check 'CSRF ditolak tanpa token' {
    try { Invoke-RestMethod "$BaseUrl/api/v1/auth/logout" -Method Post -Headers @{ Origin = $Origin } -WebSession $session | Out-Null; throw 'mutasi tanpa CSRF diterima' }
    catch [System.Net.WebException] { if ([int]$_.Exception.Response.StatusCode -ne 403) { throw "status $([int]$_.Exception.Response.StatusCode)" } }
    '(403)'
}
Check 'logout' { Invoke-RestMethod "$BaseUrl/api/v1/auth/logout" -Method Post -Headers @{ Origin = $Origin; 'X-CSRF-Token' = $script:csrf } -WebSession $session | Out-Null; '' }

if ($failed -gt 0) { Write-Host "Smoke gagal: $failed" -ForegroundColor Red; exit 1 }
Write-Host 'Smoke lulus.' -ForegroundColor Green
