SKILL_NAME := kosis
SKILL_SRC  := $(CURDIR)
DEPLOY_DIRS := $(HOME)/.claude/skills/$(SKILL_NAME) \
               $(HOME)/.codex/skills/$(SKILL_NAME)

ifeq ($(OS),Windows_NT)
    PLATFORM := windows
    RM_RF    := powershell -Command "Remove-Item -Recurse -Force -ErrorAction SilentlyContinue"
    MKDIR_P  := powershell -Command "New-Item -ItemType Directory -Force -Path"
    CP_R     := powershell -Command "Copy-Item -Recurse -Force"
else
    PLATFORM := $(shell uname -s | tr A-Z a-z)
    RM_RF    := rm -rf
    MKDIR_P  := mkdir -p
    CP_R     := cp -R
endif

.PHONY: help dev-link deploy unlink check

help:
	@echo "KOSIS 스킬 배포 (skills/kosis/Makefile)"
	@echo ""
	@echo "  make dev-link   ~/.claude, ~/.codex 에 symlink 생성 (개발용, 수정 즉시 반영)"
	@echo "  make deploy     ~/.claude, ~/.codex 에 파일 복사 (배포용)"
	@echo "  make unlink     symlink/배포본 제거"
	@echo "  make check      설치 상태 점검"

# 개발 모드: symlink (수정 즉시 반영, Windows에서 실패 시 copy로 fallback)
dev-link:
	@for dest in $(DEPLOY_DIRS); do \
		parent=$$(dirname "$$dest"); \
		$(MKDIR_P) "$$parent" >/dev/null 2>&1 || true; \
		$(RM_RF) "$$dest"; \
		ln -snf "$(SKILL_SRC)" "$$dest" 2>/dev/null \
			&& echo "  link  $$dest -> $(SKILL_SRC)" \
			|| { echo "  symlink 실패 → copy로 fallback: $$dest"; \
			     $(MAKE) --no-print-directory _copy-one DEST="$$dest"; }; \
	done
	@echo "✓ dev-link 완료 (소스 수정 즉시 반영)"

# 배포 모드: 파일 복사
deploy: unlink
	@for dest in $(DEPLOY_DIRS); do \
		$(MAKE) --no-print-directory _copy-one DEST="$$dest"; \
	done
	@echo "✓ 배포 완료"

_copy-one:
	@$(MKDIR_P) "$(DEST)" >/dev/null
	@cp "$(SKILL_SRC)/SKILL.md"    "$(DEST)/SKILL.md"
	@cp "$(SKILL_SRC)/LEARNINGS.md" "$(DEST)/LEARNINGS.md"
	@cp "$(SKILL_SRC)/VERSION"     "$(DEST)/VERSION"
	@$(RM_RF) "$(DEST)/references" && $(CP_R) "$(SKILL_SRC)/references" "$(DEST)/references"
	@$(RM_RF) "$(DEST)/templates"  && $(CP_R) "$(SKILL_SRC)/templates"  "$(DEST)/templates"
	@$(RM_RF) "$(DEST)/scripts"    && $(CP_R) "$(SKILL_SRC)/scripts"    "$(DEST)/scripts"
	@$(MKDIR_P) "$(DEST)/apps" >/dev/null
	@echo "  copy  $(DEST)"

# 정리
unlink:
	@for dest in $(DEPLOY_DIRS); do \
		$(RM_RF) "$$dest" && echo "  remove $$dest" || true; \
	done

# 상태 점검
check:
	@echo "=== KOSIS 스킬 설치 상태 ==="
	@for dest in $(DEPLOY_DIRS); do \
		if [ -L "$$dest" ]; then \
			echo "  [LINK] $$dest -> $$(readlink $$dest)"; \
		elif [ -d "$$dest" ]; then \
			echo "  [COPY] $$dest"; \
		else \
			echo "  [MISS] $$dest (미설치)"; \
		fi; \
	done
	@echo ""
	@echo "=== 바이너리 상태 ==="
	@_os=$$(uname -s | tr A-Z a-z); \
	_arch=$$(uname -m); \
	case "$$_arch" in x86_64|amd64) _arch=amd64 ;; arm64|aarch64) _arch=arm64 ;; esac; \
	for dest in $(DEPLOY_DIRS); do \
		bin="$$dest/apps/kosis-$$_os-$$_arch"; \
		if [ -x "$$bin" ]; then echo "  [OK]   $$bin"; \
		else echo "  [MISS] $$bin (설치 필요: sh scripts/install-binary.sh)"; fi; \
	done
