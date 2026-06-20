# KOSIS 제거 스크립트 (Windows PowerShell)
# 사용법: irm https://raw.githubusercontent.com/clazic/kosis/master/scripts/uninstall.ps1 | iex
#         .\uninstall.ps1 [-Purge] [-Yes]
#
# 옵션:
#   -Purge   설정·데이터(%USERPROFILE%\.kosis)까지 삭제
#   -Yes     확인 프롬프트 없이 바로 실행
#
# 환경변수:
#   KOSIS_PURGE=1   -Purge와 동일
#   KOSIS_YES=1     -Yes와 동일
param(
  [switch]$Purge,
  [switch]$Yes
)
$ErrorActionPreference = "Stop"
try { chcp 65001 | Out-Null } catch { }  # Windows 콘솔 UTF-8 (없는 환경 무시)

# 환경변수 병합
if ($env:KOSIS_PURGE -eq "1") { $Purge = $true }
if ($env:KOSIS_YES   -eq "1") { $Yes   = $true }

$Home_    = $env:USERPROFILE
$BinDir   = Join-Path $env:LOCALAPPDATA "Programs\kosis"
$Cwd      = (Get-Location).Path

# ── 제거 대상 수집 (존재하는 것만) ──
$Candidates = @(
  $BinDir,
  (Join-Path $Home_  ".claude\skills\kosis"),
  (Join-Path $Home_  ".codex\skills\kosis"),
  (Join-Path $Cwd    ".claude\skills\kosis"),
  (Join-Path $Cwd    ".codex\skills\kosis")
)
$Targets = $Candidates | Where-Object { Test-Path $_ }

if ($Targets.Count -eq 0) {
  Write-Host "제거할 항목이 없습니다."
  exit 0
}

# ── 미리보기 출력 ──
Write-Host "다음 항목을 제거합니다:"
$Targets | ForEach-Object { Write-Host "  $_" }

# ── 확인 질문 ──
if (-not $Yes) {
  $c = Read-Host "`n계속하시겠습니까? (y/N)"
  if ($c -notmatch '^[yY]$') { Write-Host "취소됨."; exit 0 }
}

# ── config 삭제 여부 추가 질문 (purge 아닐 때) ──
if (-not $Purge) {
  $p = Read-Host "`n설정·데이터(~\.kosis)도 삭제할까요? (y/N)"
  if ($p -match '^[yY]$') { $Purge = $true }
}

# ── 삭제 실행 ──
function Remove-Target($t) {
  try {
    if (Test-Path $t) {
      Remove-Item -Recurse -Force $t
      Write-Host "  ✓ 제거: $t"
    }
  } catch {
    Write-Warning "  제거 실패(무시): $t — $_"
  }
}

$Targets | ForEach-Object { Remove-Target $_ }

# ── Windows PATH 되돌리기 ──
try {
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $newPath  = ($userPath -split ';' | Where-Object { $_ -and $_ -ne $BinDir }) -join ';'
  if ($newPath -ne $userPath) {
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "  ✓ PATH 제거: $BinDir"
  }
} catch {
  Write-Warning "  Windows PATH 되돌리기 실패(무시): $_"
}

# ── config 처리 ──
$ConfigDir  = Join-Path $Home_ ".kosis"
$ConfigNote = "(설정·데이터 보존: $ConfigDir)"
if ($Purge) {
  if (Test-Path $ConfigDir) {
    Write-Host ""
    Write-Host "⚠  설정·데이터 삭제: $ConfigDir (API 키/북마크/이력/캐시 포함)"
    Remove-Target $ConfigDir
  }
  $ConfigNote = "(설정·데이터 삭제됨)"
}

Write-Host ""
Write-Host "✓ kosis 제거 완료 $ConfigNote" -ForegroundColor Green
