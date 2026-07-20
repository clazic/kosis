package cmd

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/clazic/kosis/internal/config"
)

const updateRepo = "clazic/kosis"

var (
	updateCheckOnly bool
	updateForce     bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "kosis를 최신 버전으로 업데이트 (바이너리 + 스킬 파일)",
	Long: `kosis를 GitHub 최신 릴리스로 업데이트합니다.

바이너리(OS별)와 스킬 파일(SKILL.md, docs/, LEARNINGS.md, templates 등)을
함께 내려받아 설치된 스킬 디렉토리(~/.claude/skills/kosis, ~/.codex/skills/kosis)에 반영합니다.

사용법:
  kosis update            최신 버전으로 업데이트 (바이너리 + 스킬)
  kosis update --check    업데이트 확인만 (설치하지 않음)
  kosis update --force    같은 버전이어도 강제 재설치`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runUpdate(); err != nil {
			fmt.Fprintf(os.Stderr, "오류: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "업데이트 확인만 (설치하지 않음)")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "같은 버전이어도 강제 재설치")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate() error {
	current := appVersion
	fmt.Printf("현재 버전: %s\n", current)
	fmt.Println("최신 버전 확인 중...")

	latest, err := fetchLatestTag()
	if err != nil {
		return fmt.Errorf("최신 버전 확인 실패: %w", err)
	}
	fmt.Printf("최신 버전: %s\n", latest)

	if updateCheckOnly {
		if normalizeVer(current) == normalizeVer(latest) {
			fmt.Println("✓ 이미 최신 버전입니다.")
		} else {
			fmt.Printf("→ 새 버전 %s 사용 가능. `kosis update`로 설치하세요.\n", latest)
		}
		return nil
	}
	if !updateForce && normalizeVer(current) == normalizeVer(latest) {
		fmt.Println("✓ 이미 최신 버전입니다.")
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("홈 디렉토리 확인 실패: %w", err)
	}

	binAsset, err := binaryAssetName()
	if err != nil {
		return err
	}
	skillAsset := fmt.Sprintf("kosis-skill-%s.tar.gz", latest)
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", updateRepo, latest)

	tmp, err := os.MkdirTemp("", "kosis-update-*")
	if err != nil {
		return fmt.Errorf("임시 디렉토리 생성 실패: %w", err)
	}
	defer os.RemoveAll(tmp)

	skillTar := filepath.Join(tmp, "skill.tar.gz")
	binTmp := filepath.Join(tmp, "kosis-bin")

	fmt.Println("  스킬 파일 다운로드 중...")
	if err := downloadFile(base+"/"+skillAsset, skillTar); err != nil {
		return fmt.Errorf("스킬 파일 다운로드 실패: %w", err)
	}
	fmt.Printf("  바이너리 다운로드 중 (%s)...\n", binAsset)
	if err := downloadFile(base+"/"+binAsset, binTmp); err != nil {
		return fmt.Errorf("바이너리 다운로드 실패: %w", err)
	}

	// ── 현재 실행 바이너리 경로로 교체 ──
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("실행 바이너리 경로 확인 실패: %w", err)
	}
	// symlink 해소
	if resolved, rerr := filepath.EvalSymlinks(exePath); rerr == nil {
		exePath = resolved
	}
	fmt.Printf("  바이너리 교체 중 (%s)...\n", exePath)
	if err := replaceBinary(binTmp, exePath); err != nil {
		return fmt.Errorf("바이너리 교체 실패: %w", err)
	}
	fmt.Printf("  ✓ 바이너리: %s\n", exePath)

	// ── 스킬 갱신: global + cwd project 모두 ──
	skillDirs := collectSkillDirs(home)
	for _, dest := range skillDirs {
		if err := extractTarGz(skillTar, dest); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ 스킬 갱신 실패(%s): %v\n", dest, err)
			continue
		}
		fmt.Printf("  ✓ 스킬: %s\n", dest)
	}

	fmt.Printf("✓ kosis %s → %s 업데이트 완료 (바이너리 + 스킬 파일)\n", current, latest)
	if runtime.GOOS == "windows" {
		fmt.Println("  (Windows) 새 바이너리는 다음 실행부터 적용됩니다. 이전 버전은 *.old로 보관됩니다.")
	}

	// 업데이트 완료 후 캐시 갱신
	_ = saveUpdateCache(latest)
	return nil
}

// collectSkillDirs 존재하는 스킬 디렉토리를 수집합니다 (global + cwd project).
func collectSkillDirs(home string) []string {
	candidates := []string{
		filepath.Join(home, ".claude", "skills", "kosis"),
		filepath.Join(home, ".codex", "skills", "kosis"),
	}
	// cwd project 스킬
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, ".claude", "skills", "kosis"),
			filepath.Join(cwd, ".codex", "skills", "kosis"),
		)
	}
	var result []string
	for _, d := range candidates {
		if _, err := os.Stat(d); err == nil {
			result = append(result, d)
		}
	}
	// 존재하는 게 없으면 global claude 기본
	if len(result) == 0 {
		result = append(result, filepath.Join(home, ".claude", "skills", "kosis"))
	}
	return result
}

// ── 자동 업데이트 알림 ──

type updateCache struct {
	LastCheck   time.Time `json:"last_check"`
	LatestKnown string    `json:"latest_known"`
}

func updateCachePath() string {
	return filepath.Join(config.ConfigDir(), "update-check.json")
}

func loadUpdateCache() (*updateCache, error) {
	data, err := os.ReadFile(updateCachePath())
	if err != nil {
		return &updateCache{}, nil
	}
	var c updateCache
	if err := json.Unmarshal(data, &c); err != nil {
		return &updateCache{}, nil
	}
	return &c, nil
}

func saveUpdateCache(latestKnown string) error {
	c := updateCache{LastCheck: time.Now(), LatestKnown: latestKnown}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(config.ConfigDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(updateCachePath(), data, 0o600)
}

// shouldCheckUpdate 업데이트 알림을 실행해야 할지 결정합니다.
func shouldCheckUpdate() bool {
	// 명시적 비활성화
	if os.Getenv("KOSIS_NO_UPDATE_CHECK") != "" {
		return false
	}
	// config update_check=false
	cfg, err := config.Load()
	if err == nil && !cfg.UpdateCheck {
		return false
	}
	// stdout이 TTY가 아니면 스킵 (파이프/리다이렉트)
	fi, err := os.Stdout.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	// 24h 이내 이미 체크했으면 스킵
	cache, _ := loadUpdateCache()
	if time.Since(cache.LastCheck) < 24*time.Hour {
		return false
	}
	return true
}

// startBackgroundUpdateCheck PersistentPreRun에서 호출 — goroutine+타임아웃으로 비차단.
// 새 버전 발견 시 cmd 종료 후 stderr 알림을 위해 채널 반환.
var pendingUpdateNotice string
var pendingUpdateOnce sync.Once

func startBackgroundUpdateCheck() {
	if !shouldCheckUpdate() {
		return
	}
	go func() {
		done := make(chan string, 1)
		go func() {
			tag, err := fetchLatestTagWithTimeout(3 * time.Second)
			if err != nil {
				done <- ""
				return
			}
			_ = saveUpdateCache(tag)
			done <- tag
		}()
		select {
		case tag := <-done:
			if tag != "" && normalizeVer(tag) != normalizeVer(appVersion) {
				pendingUpdateOnce.Do(func() {
					pendingUpdateNotice = tag
				})
			}
		case <-time.After(3 * time.Second):
		}
	}()
}

// printUpdateNotice 명령 종료 시점에 stderr로 업데이트 알림을 출력합니다.
func printUpdateNotice() {
	if pendingUpdateNotice == "" {
		return
	}
	tag := pendingUpdateNotice
	fmt.Fprintf(os.Stderr, "\n새 버전 %s 사용 가능 (현재 %s).\n", tag, appVersion)
	fmt.Fprintf(os.Stderr, "지금 업데이트할까요? (y/N) ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		ans := strings.TrimSpace(scanner.Text())
		if strings.EqualFold(ans, "y") {
			if err := runUpdate(); err != nil {
				fmt.Fprintf(os.Stderr, "업데이트 실패: %v\n", err)
			}
			return
		}
	}
	// N 또는 무응답: 24h 침묵
	fmt.Fprintln(os.Stderr, "24시간 동안 알림을 표시하지 않습니다. (`kosis update`로 수동 업데이트)")
	_ = saveUpdateCache(tag)
}

func fetchLatestTagWithTimeout(timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", updateRepo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API 응답 코드 %d", resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return payload.TagName, nil
}

// fetchLatestTag GitHub 릴리스 API에서 최신 태그명을 가져옵니다.
func fetchLatestTag() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", updateRepo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API 응답 코드 %d", resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.TagName == "" {
		return "", fmt.Errorf("최신 릴리스 태그를 찾을 수 없습니다")
	}
	return payload.TagName, nil
}

// binaryAssetName 현재 OS/아키텍처에 맞는 릴리스 바이너리 자산명을 반환합니다.
func binaryAssetName() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		switch runtime.GOARCH {
		case "arm64":
			return "kosis-darwin-arm64", nil
		case "amd64":
			return "kosis-darwin-amd64", nil
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return "kosis-linux-amd64", nil
		case "arm64":
			return "kosis-linux-arm64", nil
		}
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "kosis-windows-amd64.exe", nil
		}
	}
	return "", fmt.Errorf("지원하지 않는 플랫폼: %s/%s (수동 설치 필요)", runtime.GOOS, runtime.GOARCH)
}

// downloadFile URL에서 파일을 받아 dest에 저장합니다 (리다이렉트 자동 추적).
func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("다운로드 응답 코드 %d (%s)", resp.StatusCode, url)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

// extractTarGz tar.gz를 dest에 추출합니다 (경로 traversal 방어, 기존 파일 덮어쓰기).
func extractTarGz(tarPath, dest string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	cleanDest := filepath.Clean(dest)
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if name == "." {
			continue
		}
		target := filepath.Join(cleanDest, name)
		// 경로 이탈(zip slip) 방어
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("안전하지 않은 경로 항목: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

// replaceBinary 새 바이너리를 dst에 설치합니다. 실행 중인 바이너리도 안전하게 교체합니다.
// Unix: dst.new로 쓴 뒤 rename(원자 교체, 실행 중 프로세스는 기존 inode 유지).
// Windows: 실행 중 .exe는 직접 교체 불가 → 기존을 .old로 옮긴 뒤 새 파일 배치.
func replaceBinary(src, dst string) error {
	tmpDst := dst + ".new"
	if err := copyFile(src, tmpDst, 0o755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(dst + ".old")
		if _, err := os.Stat(dst); err == nil {
			if err := os.Rename(dst, dst+".old"); err != nil {
				_ = os.Remove(tmpDst)
				return err
			}
		}
	}
	if err := os.Rename(tmpDst, dst); err != nil {
		_ = os.Remove(tmpDst)
		return err
	}
	return nil
}

// copyFile src를 dst로 복사하고 권한을 설정합니다.
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, perm)
}

// normalizeVer 비교용으로 "v" 접두사와 "-dirty"/빌드 메타데이터를 제거합니다.
func normalizeVer(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	return v
}
