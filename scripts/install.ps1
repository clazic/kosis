# KOSIS CLI 설치 스크립트 (Windows PowerShell)
# 사용법: irm https://raw.githubusercontent.com/clazic/kosis/master/scripts/install.ps1 | iex
param([string]$Version = "")
$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [Text.Encoding]::UTF8
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
chcp 65001 | Out-Null

$Repo        = "clazic/kosis"
$ClaudeSkill = Join-Path $env:USERPROFILE ".claude\skills\kosis"
$CodexSkill  = Join-Path $env:USERPROFILE ".codex\skills\kosis"

# 최신 버전 확인
if (-not $Version) {
  if ($env:KOSIS_VERSION) {
    $Version = $env:KOSIS_VERSION
  } else {
    $rel = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
    $Version = $rel.tag_name
  }
}
if (-not $Version) { throw "버전 정보를 가져올 수 없습니다." }

Write-Host "kosis $Version 설치 중..." -ForegroundColor Cyan

$Arch       = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { throw "32-bit 미지원" }
$SkillAsset = "kosis-skill-$Version.tar.gz"
$BinAsset   = "kosis-windows-$Arch.exe"
$SkillUrl   = "https://github.com/$Repo/releases/download/$Version/$SkillAsset"
$BinUrl     = "https://github.com/$Repo/releases/download/$Version/$BinAsset"

$Tmp = New-Item -ItemType Directory -Path (Join-Path $env:TEMP "kosis-install-$(Get-Random)")
try {
  # 1. 스킬 tarball 다운로드
  Write-Host "  스킬 파일 다운로드 중..."
  Invoke-WebRequest -Uri $SkillUrl -OutFile (Join-Path $Tmp "skill.tar.gz") -UseBasicParsing

  # 2. 스킬 파일 설치 (2곳) — tar는 Windows 10 1803+ 기본 내장
  foreach ($Dest in @($ClaudeSkill, $CodexSkill)) {
    if (Test-Path $Dest) { Remove-Item -Recurse -Force $Dest }
    New-Item -ItemType Directory -Force -Path $Dest | Out-Null
    tar -xzf (Join-Path $Tmp "skill.tar.gz") -C $Dest
    Write-Host "  ✓ 스킬 파일 설치: $Dest"
  }

  # 3. Windows 바이너리 다운로드
  Write-Host "  바이너리 다운로드 중 ($BinAsset)..."
  Invoke-WebRequest -Uri $BinUrl -OutFile (Join-Path $Tmp "kosis.exe") -UseBasicParsing

  # 4. 바이너리를 두 스킬 폴더에 배치
  foreach ($Dest in @($ClaudeSkill, $CodexSkill)) {
    $AppsDir = Join-Path $Dest "apps"
    New-Item -ItemType Directory -Force -Path $AppsDir | Out-Null
    Copy-Item (Join-Path $Tmp "kosis.exe") (Join-Path $AppsDir "kosis-windows-$Arch.exe") -Force
  }
} finally {
  Remove-Item -Recurse -Force $Tmp
}

Write-Host ""
Write-Host "✓ kosis $Version 설치 완료" -ForegroundColor Green
Write-Host "  한글 깨짐 시: chcp 65001 실행"

# API 키 안내
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
