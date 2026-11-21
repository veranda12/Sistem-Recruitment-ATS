<#
.SYNOPSIS
  Drill restore ke lingkungan kosong: pulihkan backup ke database baru (*_test), salin objek ke direktori baru,
  lalu verifikasi satu dokumen, satu receipt ujian, dan satu histori workflow. Mencatat durasi tiap langkah.
.EXAMPLE
  powershell -File ops\windows\restore-drill.ps1 -BackupDir D:\backup\ats\ats-ats_e2e-20261121-130000 -TargetDatabaseUrl 'postgres://pos:SANDI@localhost:5432/ats_restore_test?sslmode=disable'
#>
param([Parameter(Mandatory = $true)][string]$BackupDir, [Parameter(Mandatory = $true)][string]$TargetDatabaseUrl, [string]$RestoreObjectDir = '')
. "$PSScriptRoot\common.ps1"
$db = Get-DbParts $TargetDatabaseUrl
if ($db.Db -notlike '*_test') { throw 'Database target drill harus berakhiran _test.' }
if (-not $RestoreObjectDir) { $RestoreObjectDir = Join-Path $DataDir "restore-objects-$((Get-Date).ToString('HHmmss'))" }
$report = [ordered]@{}
$total = [Diagnostics.Stopwatch]::StartNew()
$env:PGPASSWORD = $db.Pass
function Q([string]$sql) { return (& psql -h $db.Host -p $db.Port -U $db.User -d $db.Db -tAc $sql | Out-String).Trim() }

try {
    Write-Step 'Verifikasi manifest backup'
    $m = Get-Content -Raw (Join-Path $BackupDir 'manifest.json') | ConvertFrom-Json
    foreach ($f in $m.files) {
        $h = (Get-FileHash -Algorithm SHA256 (Join-Path $BackupDir $f.path)).Hash.ToLower()
        if ($h -ne $f.sha256) { throw "Checksum berbeda: $($f.path)" }
    }
    Write-Ok "$($m.files.Count) berkas cocok dengan manifest (backup $($m.createdAt), LSN $($m.walLsnAtEnd))"

    Write-Step "Pulihkan database ke $($db.Db) (kosong)"
    $sw = [Diagnostics.Stopwatch]::StartNew()
    & dropdb -h $db.Host -p $db.Port -U $db.User --if-exists $db.Db
    & createdb -h $db.Host -p $db.Port -U $db.User -E UTF8 -T template0 $db.Db
    & pg_restore -h $db.Host -p $db.Port -U $db.User -d $db.Db --no-owner --exit-on-error (Join-Path $BackupDir 'db.dump')
    if ($LASTEXITCODE -ne 0) { throw 'pg_restore gagal' }
    $report['restore_db_seconds'] = [math]::Round($sw.Elapsed.TotalSeconds, 1)

    Write-Step 'Pulihkan objek'
    $sw.Restart()
    New-Item -ItemType Directory -Force $RestoreObjectDir | Out-Null
    Copy-Item -Recurse -Force (Join-Path $BackupDir 'objects\*') $RestoreObjectDir
    $report['restore_objects_seconds'] = [math]::Round($sw.Elapsed.TotalSeconds, 1)

    Write-Step 'Verifikasi isi'
    $report['schema_version'] = Q 'select version from schema_migrations'
    $report['users'] = Q 'select count(*) from users'
    $report['applications'] = Q 'select count(*) from job_applications'

    # 1) Dokumen: berkas ada dan SHA-256 sama dengan yang tercatat saat unggah.
    $doc = Q "select id||'|'||object_key||'|'||encode(sha256,'hex') from documents where deleted_at is null and scan_status='clean' order by created_at limit 1"
    if ($doc) {
        $id, $key, $sha = $doc.Split('|')
        $path = Join-Path $RestoreObjectDir ($key.Replace('/', '\'))
        if (-not (Test-Path $path)) { throw "Berkas dokumen $id tidak ada setelah restore ($key)" }
        $h = (Get-FileHash -Algorithm SHA256 $path).Hash.ToLower()
        if ($h -ne $sha) { throw "Isi dokumen $id berbeda dari sidik saat unggah" }
        Write-Ok "Dokumen $id terbaca dan SHA-256 cocok"
        $report['document_check'] = 'lulus'
    } else { Write-Warn2 'Tidak ada dokumen bersih di backup'; $report['document_check'] = 'tidak ada data' }

    # 2) Receipt ujian: digest jawaban = SHA-256 snapshot beku; revisi final sama.
    $rc = Q "select receipt_code||'|'||(answers_digest = sha256(convert_to(snapshot::text,'UTF8')))::text||'|'||final_revision from exam_submissions order by submitted_at limit 1"
    if ($rc) {
        $code, $same, $rev = $rc.Split('|')
        $snapRev = Q "select (snapshot->>'finalRevision') from exam_submissions where receipt_code='$code'"
        if ($snapRev -ne $rev) { throw "Receipt ${code}: revisi snapshot berbeda" }
        if ($same -ne 't' -and $same -ne 'true') { throw "Receipt ${code}: digest jawaban tidak cocok dengan snapshot yang dipulihkan" }
        Write-Ok "Receipt $code dipulihkan; digest jawaban cocok dengan snapshot (revisi final $rev)"
        $report['receipt_check'] = "lulus ($code)"
    } else { Write-Warn2 'Tidak ada receipt di backup'; $report['receipt_check'] = 'tidak ada data' }

    # 3) Histori workflow: tahap terakhir di histori = tahap lamaran saat ini, versi workflow masih ada.
    $wf = Q "select a.id||'|'||a.stage_key||'|'||(select t.to_stage from transition_history t where t.application_id=a.id order by t.id desc limit 1)||'|'||(select count(*) from transition_history t where t.application_id=a.id) from job_applications a where exists (select 1 from transition_history t where t.application_id=a.id and t.trigger in ('event','approval')) limit 1"
    if ($wf) {
        $aid, $stage, $last, $n = $wf.Split('|')
        if ($stage -ne $last) { throw "Histori workflow lamaran ${aid} tidak konsisten ($stage vs $last)" }
        Write-Ok "Histori workflow lamaran $aid konsisten ($n transisi, tahap $stage)"
        $report['workflow_check'] = "lulus ($n transisi)"
    } else { Write-Warn2 'Tidak ada histori workflow berbasis event'; $report['workflow_check'] = 'tidak ada data' }
    $report['audit_rows'] = Q 'select count(*) from audit_logs'
} finally { Remove-Item Env:\PGPASSWORD -ErrorAction SilentlyContinue }

$report['total_seconds'] = [math]::Round($total.Elapsed.TotalSeconds, 1)
$report | ConvertTo-Json | Set-Content -Encoding UTF8 (Join-Path $BackupDir 'restore-drill-report.json')
Write-Host ''
$report.GetEnumerator() | ForEach-Object { Write-Host ("  {0,-26} {1}" -f $_.Key, $_.Value) }
