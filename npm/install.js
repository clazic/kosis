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

function getSkillDirs() {
  const home = os.homedir();
  return [
    path.join(home, ".claude", "skills", "kosis"),
    path.join(home, ".codex",  "skills", "kosis"),
  ];
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

function createSymlink(target, linkPath) {
  try {
    if (fs.existsSync(linkPath)) fs.unlinkSync(linkPath);
    fs.symlinkSync(target, linkPath);
  } catch {
    // symlink 실패 시 무시
  }
}

async function main() {
  const { platform, goos, goarch, ext, binArtifact, skillArtifact } = getPlatformInfo();
  const tag      = `v${VERSION}`;
  const baseUrl  = `https://github.com/${REPO}/releases/download/${tag}`;
  const skillUrl = `${baseUrl}/${skillArtifact}`;
  const binUrl   = `${baseUrl}/${binArtifact}`;
  const skillDirs = getSkillDirs();
  const tmpDir   = fs.mkdtempSync(path.join(os.tmpdir(), "kosis-install-"));

  try {
    // 1. 스킬 tarball 다운로드 및 설치
    console.log(`kosis v${VERSION} 스킬 파일 설치 중...`);
    const skillData = await download(skillUrl);
    const tarPath   = path.join(tmpDir, skillArtifact);
    fs.writeFileSync(tarPath, skillData);

    for (const dest of skillDirs) {
      if (fs.existsSync(dest)) fs.rmSync(dest, { recursive: true, force: true });
      extractTarGz(tarPath, dest);
      console.log(`  ✓ 스킬 설치: ${dest}`);
    }

    // 2. OS별 바이너리 다운로드
    console.log(`바이너리 다운로드 중 (${binArtifact})...`);
    const binData = await download(binUrl);
    const tmpBin  = path.join(tmpDir, `kosis${ext}`);
    fs.writeFileSync(tmpBin, binData);
    if (platform !== "win32") fs.chmodSync(tmpBin, 0o755);

    // 3. 각 스킬 폴더 apps/ 에 바이너리 배치 (스킬 탐색용: kosis-<os>-<arch>[.exe])
    for (const dest of skillDirs) {
      const appsDir = path.join(dest, "apps");
      fs.mkdirSync(appsDir, { recursive: true });
      const skillBinName = `kosis-${goos}-${goarch}${ext}`;
      const skillBinDest = path.join(appsDir, skillBinName);
      fs.copyFileSync(tmpBin, skillBinDest);
      if (platform !== "win32") fs.chmodSync(skillBinDest, 0o755);
    }

    // 4. npm 런처용: bin/kosis[.exe] (패키지 내부)
    const npmBinDir = path.join(__dirname, "bin");
    fs.mkdirSync(npmBinDir, { recursive: true });
    const npmBinDest = path.join(npmBinDir, `kosis${ext}`);
    fs.copyFileSync(tmpBin, npmBinDest);
    if (platform !== "win32") fs.chmodSync(npmBinDest, 0o755);

    // 5. Unix: ~/.local/bin/kosis → 첫 번째 스킬 폴더 바이너리 symlink
    if (platform !== "win32") {
      const localBin  = path.join(os.homedir(), ".local", "bin");
      fs.mkdirSync(localBin, { recursive: true });
      const binTarget = path.join(skillDirs[0], "apps", `kosis-${goos}-${goarch}`);
      createSymlink(binTarget, path.join(localBin, "kosis"));
    }

    console.log(`\n✓ kosis v${VERSION} 설치 완료!`);

    // 5. API 키 안내
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
