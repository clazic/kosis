#!/usr/bin/env node
"use strict";
// preuninstall 훅 — npm uninstall @clazic/kosis 시 자동 실행 (비대화형)
// 실패해도 throw 금지 — npm 제거 흐름을 막지 않음

const fs = require("fs");
const path = require("path");
const os = require("os");
const { spawnSync } = require("child_process");

function tryRemove(target) {
  try {
    if (!fs.existsSync(target)) return;
    const stat = fs.statSync(target);
    if (stat.isDirectory()) {
      fs.rmSync(target, { recursive: true, force: true });
    } else {
      fs.unlinkSync(target);
    }
    console.log(`  ✓ 제거: ${target}`);
  } catch (err) {
    console.warn(`  ⚠ 제거 실패(무시): ${target} — ${err.message}`);
  }
}

function removeWindowsPathEntry(binDir) {
  try {
    const result = spawnSync(
      "powershell",
      [
        "-NoProfile", "-NonInteractive", "-Command",
        `$p=[Environment]::GetEnvironmentVariable('Path','User');` +
        `$n=($p -split ';' | Where-Object { $_ -and $_ -ne '${binDir}' }) -join ';';` +
        `[Environment]::SetEnvironmentVariable('Path',$n,'User');` +
        `Write-Host 'PATH 제거: ${binDir}'`,
      ],
      { shell: true, stdio: "pipe" }
    );
    if (result.stdout) {
      const out = result.stdout.toString().trim();
      if (out) console.log(`  ${out}`);
    }
  } catch (err) {
    console.warn(`  ⚠ Windows PATH 되돌리기 실패(무시): ${err.message}`);
  }
}

function main() {
  console.log("kosis 제거 중...");

  const home = os.homedir();

  if (process.platform === "win32") {
    // Windows: %LOCALAPPDATA%\Programs\kosis 폴더 전체
    const binDir = path.join(
      process.env.LOCALAPPDATA || path.join(home, "AppData", "Local"),
      "Programs", "kosis"
    );
    tryRemove(binDir);
    removeWindowsPathEntry(binDir);
  } else {
    // Unix: ~/.local/bin/kosis
    tryRemove(path.join(home, ".local", "bin", "kosis"));
  }

  // 스킬 global 제거 (~/.claude/skills/kosis, ~/.codex/skills/kosis)
  tryRemove(path.join(home, ".claude", "skills", "kosis"));
  tryRemove(path.join(home, ".codex",  "skills", "kosis"));

  // project 스킬은 cwd가 불확실하므로 건드리지 않음
  console.log("  (프로젝트 스킬은 각 프로젝트 폴더에서 직접 제거하세요)");

  // config는 절대 자동 삭제 안 함
  console.log("");
  console.log("설정은 ~/.kosis에 보존됨. 완전 삭제는 uninstall.sh --purge");
  console.log("✓ kosis 제거 완료");
}

try {
  main();
} catch (err) {
  // 훅 실패가 npm 제거를 막지 않도록 throw 하지 않음
  console.warn(`  ⚠ 제거 훅 오류(무시): ${err.message}`);
}
