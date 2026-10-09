# kosis quick --ai 예시 10가지를 순서대로 실행합니다 (Windows PowerShell).
#
# 사용법:
#   .\scripts\quick-ai-examples.ps1            # 하나씩 실행, 매번 확인(y/n) — 확인한 선택은 학습됨
#   .\scripts\quick-ai-examples.ps1 -Yes       # 확인 없이 연속 실행 (학습하지 않음)
#   .\scripts\quick-ai-examples.ps1 -Only 7    # 7번만 실행
#
# 환경변수: KOSIS_BIN(kosis 경로), HERMES_API_KEY, TYPESAFE_API_KEY
# 한글이 깨지면 먼저 chcp 65001 실행.
param(
    [switch]$Yes,
    [int]$Only = 0
)

$Kosis = if ($env:KOSIS_BIN) { $env:KOSIS_BIN } else { "kosis" }
if (-not (Get-Command $Kosis -ErrorAction SilentlyContinue)) {
    Write-Host "kosis 를 찾을 수 없습니다. 설치하거나 KOSIS_BIN 으로 경로를 지정하세요."
    exit 1
}
Write-Host "사용 바이너리: $((Get-Command $Kosis).Source) ($(& $Kosis --version))"
Write-Host "※ --ai 의 hermes 해석 + Jev 선택 + 학습은 v0.5.7 이상에서 동작합니다.`n"

$Examples = @(
    @("작년까지 최근 5년간 울산에서 태어난 아기 수", '상대 기간("작년") 해석 + 지역 + 일상어→출생아수'),
    @("서울 아파트 매매가격지수 최근 6개월", "규칙 사전에 없는 통계, 월간 표, 유일한 값은 [유일]로 자동 확정"),
    @("부산 소비자물가지수 2024년 월별", "연도 범위를 월 형식으로 변환 (202401~202412)"),
    @("전국 실업률 최근 8분기", "요청 주기(분기)가 없는 표면 연간으로 환산 (8분기→2년). 신뢰도가 낮으면 ⚠ 표시"),
    @("경기도 혼인 건수 2020년부터", '"~부터"(끝 없음)를 올해까지로 해석'),
    @("제주 고용률 최근 3년", "분류축 2개(시도·성별)를 한 번에 병렬 선택"),
    @("대구 65세 이상 인구 최근 5년", '교정 학습 연습: 추계인구 표를 고르면 n → "통계표가 틀림"으로 정답을 가르쳐 보세요'),
    @("2023년 시도별 합계출산율 중 세종", "단일 연도 + 지역 하나 고르기"),
    @("전국 사업체 수 최근 3년", "타 기관(고용노동부) 표, 분류축 3개(산업·성별·규모)"),
    @("강원 인구 순이동 2022~2024", "연속 범위, 음수 값 포함 결과")
)

for ($i = 0; $i -lt $Examples.Count; $i++) {
    $no = $i + 1
    if ($Only -ne 0 -and $Only -ne $no) { continue }
    $req, $desc = $Examples[$i]
    Write-Host ("═" * 60)
    Write-Host " [$no/$($Examples.Count)] $req"
    Write-Host "   ↳ $desc"
    Write-Host ("═" * 60)
    if ($Yes) { & $Kosis q $req --ai --yes } else { & $Kosis q $req --ai }
    if ($Only -eq 0 -and $no -lt $Examples.Count -and -not $Yes) {
        $answer = Read-Host "`n▶ Enter: 다음 예시 / q: 종료"
        if ($answer -eq "q") { break }
    }
    Write-Host ""
}
