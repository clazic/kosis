# KOSIS CLI 멀티플랫폼 빌드
# 산출물: src/bin/ 과 스킬 루트 ../apps/ 두 곳에 동일 배치
# 파일명 컨벤션: kosis-<os>-<arch>[.exe] (release.yml / install-binary.sh 와 일치)
#
# 사용법:
#   make build   # 5개 플랫폼 전부 빌드 후 bin/ 과 ../apps/ 에 배치
#   make native  # 현재 플랫폼(darwin/arm64, CGO=1)만 빌드
#   make clean   # 산출물 삭제

BIN  := bin
APPS := ../apps

.PHONY: build native mcp clean test sync-rules

# 배포 규칙(../references/rules.yaml)을 바이너리 내장용 사본으로 복사 — Go embed 는 모듈 밖 파일을 못 읽는다
sync-rules:
	@cp ../references/rules.yaml internal/rules/embedded_rules.yaml

build: sync-rules
	@echo "▶ 멀티플랫폼 빌드 시작..."
	@mkdir -p $(BIN) $(APPS)
	CGO_ENABLED=1 GOOS=darwin  GOARCH=arm64 go build -o $(BIN)/kosis-darwin-arm64     .
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -o $(BIN)/kosis-darwin-amd64     .
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o $(BIN)/kosis-linux-amd64      .
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -o $(BIN)/kosis-linux-arm64      .
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(BIN)/kosis-windows-amd64.exe .
	@cp $(BIN)/kosis-darwin-arm64 $(BIN)/kosis-darwin-amd64 $(BIN)/kosis-linux-amd64 $(BIN)/kosis-linux-arm64 $(BIN)/kosis-windows-amd64.exe $(APPS)/
	@echo "✅ 빌드 완료 — src/bin/ 과 ../apps/ 에 배치됨"
	@ls -lh $(BIN)/kosis-*

native: sync-rules
	@mkdir -p $(BIN) $(APPS)
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -o $(BIN)/kosis-darwin-arm64 .
	@cp $(BIN)/kosis-darwin-arm64 $(APPS)/
	@echo "✅ 네이티브 빌드 완료"

# MCP 서버 (stdio) — 현재 플랫폼만. 릴리스 자산 kosis-mcp-<os>-<arch> 는 CI 가 만든다
mcp: sync-rules
	@mkdir -p $(BIN)
	go build -o $(BIN)/kosis-mcp-$$(go env GOOS)-$$(go env GOARCH) ./cmd/kosis-mcp
	@echo "✅ MCP 서버 빌드 완료 — $(BIN)/kosis-mcp-$$(go env GOOS)-$$(go env GOARCH)"

clean:
	rm -f $(BIN)/kosis-* $(APPS)/kosis-*
	@echo "✅ 산출물 삭제 완료"

test: sync-rules
	go test -count=1 ./...  # rules.yaml(모듈 밖) 변경을 캐시가 못 봄

