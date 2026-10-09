# kosis quick --ai 학습 평가 (Windows PowerShell)
#
# 어려운 혼합 질문 n개를 여러 라운드 돌려 질문별 점수(100점)를 매기고,
# 라운드 사이에 사람의 교정을 정답으로 흉내 내 학습시켜 점수가 어떻게 오르는지 보여줍니다.
# 실제 학습 파일(~/.kosis/learnings.yaml)과 스킬 문서는 바뀌지 않습니다 (임시 학습 파일 사용).
#
# 사용법:
#   .\scripts\quick-ai-eval.ps1                  # 전체 질문, 3라운드
#   .\scripts\quick-ai-eval.ps1 -Limit 10        # 앞에서 10개만
#   .\scripts\quick-ai-eval.ps1 -Limit 10 -Rounds 5
#
# 환경변수: KOSIS_BIN, HERMES_API_KEY, TYPESAFE_API_KEY. 한글이 깨지면 먼저 chcp 65001.
param(
    [int]$Limit = 0,
    [int]$Rounds = 3
)

$Dir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Questions = Join-Path $Dir "quick-ai-eval.yaml"
$TmpDir = $null

$Kosis = $env:KOSIS_BIN
if (-not $Kosis) {
    $src = Join-Path (Split-Path -Parent $Dir) "src"
    if ((Test-Path (Join-Path $src "go.mod")) -and (Get-Command go -ErrorAction SilentlyContinue)) {
        # 저장소에서 실행하면 현재 소스로 빌드 (quick-eval 은 v0.5.8 이상에만 있음)
        $TmpDir = Join-Path ([System.IO.Path]::GetTempPath()) (".kosis-eval-bin-" + [guid]::NewGuid().ToString("N"))
        New-Item -ItemType Directory -Path $TmpDir | Out-Null
        if ($IsWindows -or $env:OS -eq "Windows_NT") { attrib +h $TmpDir 2>$null }
        $Kosis = Join-Path $TmpDir "kosis.exe"
        Write-Host "🔨 현재 소스로 kosis 빌드 중..."
        Push-Location $src
        go build -o $Kosis .
        $ok = $LASTEXITCODE -eq 0
        Pop-Location
        if (-not $ok) { Remove-Item -Recurse -Force $TmpDir; exit 1 }
    } else {
        $Kosis = "kosis"
    }
}

try {
    & $Kosis quick-eval --help *> $null
    if ($LASTEXITCODE -ne 0) {
        Write-Host "이 kosis 에는 quick-eval 이 없습니다. v0.5.8 이상으로 업데이트하거나 저장소에서 실행하세요."
        exit 1
    }
    $env:KOSIS_NO_UPDATE_CHECK = "1"
    & $Kosis quick-eval $Questions --limit $Limit --rounds $Rounds
} finally {
    if ($TmpDir) { Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue }
}
