#!/usr/bin/env node
"use strict";

const https = require("https");
const http = require("http");
const fs = require("fs");
const path = require("path");
const os = require("os");
const { spawnSync } = require("child_process");

const VERSION = require("./package.json").version;
const REPO = "clazic/kosis";

function getPlatformInfo() {
  const platform = os.platform();
  const arch = os.arch();

  const platformMap = { darwin: "darwin", linux: "linux", win32: "windows" };
  const archMap = { x64: "amd64", arm64: "arm64" };

  const goos = platformMap[platform];
  const goarch = archMap[arch];

  if (!goos || !goarch) {
    throw new Error(
      `지원하지 않는 플랫폼: ${platform}/${arch}\n` +
      "지원: darwin/arm64, darwin/amd64, linux/amd64, linux/arm64, windows/amd64"
    );
  }

  const ext = platform === "win32" ? ".exe" : "";
  const binArtifact  = `kosis-${goos}-${goarch}${ext}`;
  const skillArtifact = `kosis-skill-v${VERSION}.tar.gz`;

  return { platform, goos, goarch, ext, binArtifact, skillArtifact };
}

function download(url) {
  return new Promise((resolve, reject) => {
    const client = url.startsWith("https") ? https : http;
    client.get(url, { headers: { "User-Agent": "kosis-npm-installer" } }, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        return download(res.headers.location).then(resolve).catch(reject);
      }
      if (res.statusCode !== 200) {
        return reject(new Error(`다운로드 실패: HTTP ${res.statusCode} (${url})`));
      }
      const chunks = [];
      res.on("data", (chunk) => chunks.push(chunk));
      res.on("end", () => resolve(Buffer.concat(chunks)));
      res.on("error", reject);
    }).on("error", reject);
  });
}

function extractTarGz(tarPath, destDir) {
  fs.mkdirSync(destDir, { recursive: true });
  const isWin = process.platform === "win32";
  const result = spawnSync("tar", ["-xzf", tarPath, "-C", destDir], {
    shell: isWin,
    stdio: "inherit",
  });
  if (result.status !== 0) {
    throw new Error(`tar 압축 해제 실패 (exit ${result.status})`);
  }
}

// Windows PATH 등록: 사용자 환경변수에 binDir 추가 (중복 방지)
function registerWindowsPath(binDir) {
  const result = spawnSync(
    "powershell",
    [
      "-NoProfile", "-NonInteractive", "-Command",
      `$p=[Environment]::GetEnvironmentVariable('Path','User');` +
      `if($p -notlike '*${binDir}*'){` +
      `[Environment]::SetEnvironmentVariable('Path',\"$p;${binDir}\",'User');` +
      `Write-Host 'PATH 등록: ${binDir}'` +
      `}`,
    ],
    { shell: true, stdio: "pipe" }
  );
  if (result.stdout) {
    const out = result.stdout.toString().trim();
    if (out) console.log(`  ${out} (새 터미널부터 적용)`);
  }
}

async function main() {
  const { platform, goos, goarch, ext, binArtifact, skillArtifact } = getPlatformInfo();
  const tag       = `v${VERSION}`;
  const baseUrl   = `https://github.com/${REPO}/releases/download/${tag}`;
  const skillUrl  = `${baseUrl}/${skillArtifact}`;
  const binUrl    = `${baseUrl}/${binArtifact}`;
  const tmpDir    = fs.mkdtempSync(path.join(os.tmpdir(), "kosis-install-"));

  try {
    // ── 1. CLI 바이너리 다운로드 및 rename 설치 ──
    console.log(`바이너리 다운로드 중 (${binArtifact})...`);
    const binData = await download(binUrl);
    const tmpBin  = path.join(tmpDir, `kosis${ext}`);
    fs.writeFileSync(tmpBin, binData);
    if (platform !== "win32") fs.chmodSync(tmpBin, 0o755);

    if (platform === "win32") {
      // Windows: %LOCALAPPDATA%\Programs\kosis\kosis.exe
      const binDir  = path.join(process.env.LOCALAPPDATA || path.join(os.homedir(), "AppData", "Local"), "Programs", "kosis");
      const binDest = path.join(binDir, "kosis.exe");
      fs.mkdirSync(binDir, { recursive: true });
      fs.renameSync(tmpBin, binDest);
      console.log(`  ✓ CLI: ${binDest}`);
      registerWindowsPath(binDir);
    } else {
      // Unix: ~/.local/bin/kosis
      const localBin  = path.join(os.homedir(), ".local", "bin");
      const binDest   = path.join(localBin, "kosis");
      fs.mkdirSync(localBin, { recursive: true });
      fs.renameSync(tmpBin, binDest);
      fs.chmodSync(binDest, 0o755);
      console.log(`  ✓ CLI: ${binDest}`);

      // PATH 안내 (자동 수정 X)
      const pathEnv = process.env.PATH || "";
      if (!pathEnv.split(":").includes(localBin)) {
        console.log("");
        console.log("PATH에 ~/.local/bin 추가가 필요합니다:");
        console.log('  echo \'export PATH="$HOME/.local/bin:$PATH"\' >> ~/.zshrc   # zsh');
        console.log('  echo \'export PATH="$HOME/.local/bin:$PATH"\' >> ~/.bashrc  # bash');
        console.log("  새 터미널을 열거나 위 명령 실행 후 source ~/.zshrc");
      }
    }

    // ── 2. 스킬 tarball 다운로드 및 설치 ──
    console.log(`\nkosis v${VERSION} 스킬 파일 설치 중...`);
    const skillData = await download(skillUrl);
    const tarPath   = path.join(tmpDir, skillArtifact);
    fs.writeFileSync(tarPath, skillData);

    // 대상 결정: env KOSIS_TARGET (기본 claude)
    const target = (process.env.KOSIS_TARGET || "claude").toLowerCase();

    function installSkill(tool) {
      // 범위: env KOSIS_<TOOL>_SCOPE (기본 global)
      const scopeEnv = process.env[`KOSIS_${tool.toUpperCase()}_SCOPE`] || "global";
      const scope    = scopeEnv === "project" ? "project" : "global";
      const base     = scope === "project" ? process.cwd() : os.homedir();
      const dest     = path.join(base, `.${tool}`, "skills", "kosis");
      if (fs.existsSync(dest)) fs.rmSync(dest, { recursive: true, force: true });
      extractTarGz(tarPath, dest);
      console.log(`  ✓ 스킬(${tool}, ${scope}): ${dest}`);
    }

    if (target === "both") {
      installSkill("claude");
      installSkill("codex");
    } else if (target === "codex") {
      installSkill("codex");
    } else {
      installSkill("claude");
    }

    console.log(`\n✓ kosis v${VERSION} 설치 완료!`);

    // ── 3. API 키 안내 ──
    const cfgPath = path.join(os.homedir(), ".kosis", "config.yaml");
    if (!fs.existsSync(cfgPath) && !process.env.KOSIS_API_KEY) {
      console.log("\n─────────────────────────────────────────────");
      console.log(" API 키 설정이 필요합니다:");
      console.log("   kosis config setup        (대화형, 권장)");
      console.log("   kosis config set-key KEY  (직접 입력)");
      console.log(" 키 발급: https://kosis.kr/openapi/");
      console.log("─────────────────────────────────────────────");
    }
  } catch (err) {
    console.error(`\nkosis 설치 실패: ${err.message}`);
    console.error(`수동 다운로드: https://github.com/${REPO}/releases/tag/${tag}`);
    process.exit(1);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

main();
