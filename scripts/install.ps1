# KOSIS 설치 스크립트 (Windows PowerShell)
# 사용법: irm https://raw.githubusercontent.com/clazic/kosis/master/scripts/install.ps1 | iex
#
# 비대화형 환경변수:
#   KOSIS_TARGET=claude|codex|both     (기본 claude)
#   KOSIS_CLAUDE_SCOPE=global|project  (기본 global)
#   KOSIS_CODEX_SCOPE=global|project   (기본 global)
#   KOSIS_VERSION=vX.Y.Z               (기본: 최신 릴리스)
param([string]$Version = "")
$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [Text.Encoding]::UTF8
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
try { chcp 65001 | Out-Null } catch { }  # Windows 콘솔 UTF-8 (없는 환경 무시)

$Repo    = "clazic/kosis"
$BinDir  = Join-Path $env:LOCALAPPDATA "Programs\kosis"
$BinPath = Join-Path $BinDir "kosis.exe"

# ── 버전 확인 ──
if (-not $Version) {
  if ($env:KOSIS_VERSION) { $Version = $env:KOSIS_VERSION }
  else {
    $rel = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
    $Version = $rel.tag_name
  }
}
if (-not $Version) { throw "버전 정보를 가져올 수 없습니다." }
Write-Host "kosis $Version 설치 중..." -ForegroundColor Cyan

$Arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { throw "32-bit 미지원" }
$BinAsset   = "kosis-windows-$Arch.exe"
$SkillAsset = "kosis-skill-$Version.tar.gz"
$BinUrl     = "https://github.com/$Repo/releases/download/$Version/$BinAsset"
$SkillUrl   = "https://github.com/$Repo/releases/download/$Version/$SkillAsset"

$Tmp = New-Item -ItemType Directory -Path (Join-Path $env:TEMP "kosis-install-$(Get-Random)")
try {
  # ── 1. CLI 바이너리 → %LOCALAPPDATA%\Programs\kosis\kosis.exe ──
  Write-Host "  CLI 바이너리 다운로드 중 ($BinAsset)..."
  New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
  Invoke-WebRequest -Uri $BinUrl -OutFile $BinPath -UseBasicParsing
  Write-Host "  OK CLI: $BinPath"

  # 사용자 PATH에 등록 (없을 때만)
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if ($userPath -notlike "*$BinDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$BinDir", "User")
    Write-Host "  OK PATH 등록: $BinDir (새 터미널부터 적용)"
  }
  $env:Path = "$env:Path;$BinDir"  # 현재 세션 즉시 반영

  # ── 2. 스킬: 대상 선택 ──
  if ($env:KOSIS_TARGET) {
    $Target = $env:KOSIS_TARGET
  } else {
    Write-Host "`n설치 대상을 선택하세요:`n  1) Claude`n  2) Codex`n  3) 둘 다"
    switch (Read-Host "> ") { "1" { $Target = "claude" } "2" { $Target = "codex" } "3" { $Target = "both" } default { $Target = "claude" } }
  }

  # 범위 질문 (대상별 개별) — $Name 표시명, $EnvDefault 비대화형 기본값
  function Get-Scope($Name, $EnvDefault) {
    if ($EnvDefault) { return $(if ($EnvDefault -eq "project") { "project" } else { "global" }) }
    Write-Host "`n[$Name] 설치 범위를 선택하세요:`n  1) 전역 (모든 프로젝트)`n  2) 프로젝트 (현재 폴더)"
    switch (Read-Host "> ") { "2" { "project" } default { "global" } }
  }

  # 스킬 tar 다운로드 (한 번)
  Write-Host "  스킬 파일 다운로드 중..."
  $SkillTar = Join-Path $Tmp "skill.tar.gz"
  Invoke-WebRequest -Uri $SkillUrl -OutFile $SkillTar -UseBasicParsing

  # 스킬 설치 — $Tool: claude/codex, $Scope: global/project
  function Install-Skill($Tool, $Scope) {
    $base = if ($Scope -eq "project") { (Get-Location).Path } else { $env:USERPROFILE }
    $dest = Join-Path $base ".$Tool\skills\kosis"
    if (Test-Path $dest) { Remove-Item -Recurse -Force $dest }
    New-Item -ItemType Directory -Force -Path $dest | Out-Null
    tar -xzf $SkillTar -C $dest   # tar: Windows 10 1803+ 기본 내장
    Write-Host "  OK 스킬($Tool, $Scope): $dest"
  }

  switch ($Target) {
    "claude" { Install-Skill "claude" (Get-Scope "Claude" $env:KOSIS_CLAUDE_SCOPE) }
    "codex"  { Install-Skill "codex"  (Get-Scope "Codex"  $env:KOSIS_CODEX_SCOPE) }
    "both" {
      Install-Skill "claude" (Get-Scope "Claude" $env:KOSIS_CLAUDE_SCOPE)
      Install-Skill "codex"  (Get-Scope "Codex"  $env:KOSIS_CODEX_SCOPE)
    }
    default { throw "알 수 없는 대상: $Target" }
  }
} finally {
  Remove-Item -Recurse -Force $Tmp
}

Write-Host ""
Write-Host "OK kosis $Version 설치 완료" -ForegroundColor Green
Write-Host "  새 터미널을 열어 'kosis'를 사용하세요. 한글 깨짐 시: chcp 65001"

# ── API 키 안내 ──
$cfg = Join-Path $env:USERPROFILE ".kosis\config.yaml"
if (-not (Test-Path $cfg) -and -not $env:KOSIS_API_KEY) {
  Write-Host ""
  Write-Host "─────────────────────────────────────────────" -ForegroundColor Cyan
  Write-Host " API 키 설정이 필요합니다:"
  Write-Host "   kosis config setup        (대화형, 권장)"
  Write-Host "   kosis config set-key KEY  (직접 입력)"
  Write-Host " 키 발급: https://kosis.kr/openapi/"
  Write-Host "─────────────────────────────────────────────" -ForegroundColor Cyan
}
