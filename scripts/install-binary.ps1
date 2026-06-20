# Claude 플러그인용 — 바이너리만 다운로드하여 apps\ 에 배치
param()
$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [Text.Encoding]::UTF8
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$SkillDir = Split-Path -Parent $PSScriptRoot
$Repo = "clazic/kosis"

# 버전 읽기
$VersionFile = Join-Path $SkillDir "VERSION"
if (Test-Path $VersionFile) {
  $Version = (Get-Content $VersionFile -Raw).Trim()
} else {
  $rel = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
  $Version = $rel.tag_name
}

$Arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { throw "32-bit 미지원" }
$Asset = "kosis-windows-$Arch.exe"
$Url   = "https://github.com/$Repo/releases/download/$Version/$Asset"
$Dest  = Join-Path $SkillDir "apps\kosis-windows-$Arch.exe"

Write-Host "kosis $Version 바이너리 다운로드 중 ($Asset)..." -ForegroundColor Cyan

$Tmp = New-Item -ItemType Directory -Path (Join-Path $env:TEMP "kosis-bin-$(Get-Random)")
try {
  Invoke-WebRequest -Uri $Url -OutFile (Join-Path $Tmp "kosis.exe") -UseBasicParsing
  New-Item -ItemType Directory -Force -Path (Join-Path $SkillDir "apps") | Out-Null
  Copy-Item (Join-Path $Tmp "kosis.exe") $Dest -Force
} finally {
  Remove-Item -Recurse -Force $Tmp
}

Write-Host "✓ 바이너리 설치 완료: $Dest" -ForegroundColor Green
