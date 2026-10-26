package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/buildinfo"
)

const (
	releaseCheckURL = "https://api.github.com/repos/SeasX/seasagi/releases/latest"
	checkInterval   = 4 * time.Hour

	maxChecksumManifestSize = 1 << 20    // 校验清单最大 1MB
	maxDownloadSize         = 1 << 30    // 安装包最大 1GB，防止磁盘填满
	checksumAssetHint       = "checksum" // 资产名包含该关键词时视为 SHA-256 清单
)

type ReleaseInfo struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		// Digest 由 GitHub 计算，格式为 "sha256:<hex>"（新版本 API 才有）。
		Digest string `json:"digest"`
	} `json:"assets"`
}

type UpdateStatus struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	HasUpdate      bool   `json:"has_update"`
	ReleaseNotes   string `json:"release_notes"`
	DownloadURL    string `json:"download_url"`
	// 以下字段仅内部使用，不暴露给前端。
	DownloadSHA256 string `json:"-"` // 直连资产摘要（GitHub digest）
	ChecksumURL    string `json:"-"` // SHA-256 清单地址（无 digest 时回退）
	AssetName      string `json:"-"`
}

func CheckUpdate() (map[string]interface{}, error) {
	status, err := checkForUpdate()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"current_version": status.CurrentVersion,
		"latest_version":  status.LatestVersion,
		"has_update":      status.HasUpdate,
		"release_notes":   status.ReleaseNotes,
		"download_url":    status.DownloadURL,
	}, nil
}

func PerformUpdate() error {
	status, err := checkForUpdate()
	if err != nil {
		return fmt.Errorf("check update failed: %w", err)
	}
	if !status.HasUpdate {
		return fmt.Errorf("no update available")
	}

	if status.DownloadURL == "" {
		return openReleasesPage()
	}

	// 安全：先确定期望的 SHA-256，再下载。没有摘要/清单时直接中止，
	// 绝不安装未校验的安装包（Release 被接管等于持久化后门）。
	expectedSHA, err := resolveExpectedSHA256(status)
	if err != nil {
		return err
	}

	dmgPath, err := downloadDMG(status.DownloadURL, expectedSHA)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(dmgPath)

	if err := installDMG(dmgPath); err != nil {
		return err
	}
	return nil
}

// resolveExpectedSHA256 优先使用 GitHub 资产 digest，其次拉取 SHA-256 清单。
func resolveExpectedSHA256(status *UpdateStatus) (string, error) {
	if status.DownloadSHA256 != "" {
		return status.DownloadSHA256, nil
	}
	if status.ChecksumURL == "" {
		return "", fmt.Errorf("update aborted: release has no SHA-256 digest or checksum manifest")
	}
	manifest, err := fetchChecksumManifest(status.ChecksumURL)
	if err != nil {
		return "", fmt.Errorf("update aborted: fetch checksum manifest: %w", err)
	}
	name := status.AssetName
	if name == "" {
		name = pathBase(status.DownloadURL)
	}
	sum, ok := lookupChecksum(manifest, name)
	if !ok || sum == "" {
		return "", fmt.Errorf("update aborted: no SHA-256 entry for %s in checksum manifest", name)
	}
	return sum, nil
}

func checkForUpdate() (*UpdateStatus, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, releaseCheckURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned status %d", resp.StatusCode)
	}

	var release ReleaseInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxChecksumManifestSize)).Decode(&release); err != nil {
		return nil, err
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	hasUpdate := versionGreaterThan(latestVersion, buildinfo.Version)

	status := &UpdateStatus{
		CurrentVersion: buildinfo.Version,
		LatestVersion:  latestVersion,
		HasUpdate:      hasUpdate,
		ReleaseNotes:   release.Body,
	}
	if !hasUpdate {
		return status, nil
	}

	suffix := fmt.Sprintf("%s-%s.dmg", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "darwin" && (runtime.GOARCH == "arm64" || runtime.GOARCH == "amd64") {
		suffix = "macos.dmg"
	}
	checksumURL := ""
	for _, asset := range release.Assets {
		nameLower := strings.ToLower(asset.Name)
		if checksumURL == "" && (strings.Contains(nameLower, checksumAssetHint) ||
			nameLower == "sha256sums" || nameLower == "sha256sums.txt" || nameLower == "checksums.txt") {
			checksumURL = asset.BrowserDownloadURL
			continue
		}
		if status.DownloadURL == "" && (strings.HasSuffix(asset.Name, suffix) || strings.HasSuffix(asset.Name, ".dmg")) {
			status.DownloadURL = asset.BrowserDownloadURL
			status.AssetName = asset.Name
			status.DownloadSHA256 = parseDigest(asset.Digest)
		}
	}
	if status.DownloadURL == "" {
		status.DownloadURL = release.HTMLURL
		status.AssetName = ""
	}
	status.ChecksumURL = checksumURL
	return status, nil
}

func parseDigest(digest string) string {
	digest = strings.TrimSpace(digest)
	if strings.HasPrefix(strings.ToLower(digest), "sha256:") {
		return strings.ToLower(strings.TrimPrefix(digest, "sha256:"))
	}
	return ""
}

func versionGreaterThan(a, b string) bool {
	aParts := parseVersion(a)
	bParts := parseVersion(b)
	for i := 0; i < 3; i++ {
		if aParts[i] > bParts[i] {
			return true
		}
		if aParts[i] < bParts[i] {
			return false
		}
	}
	return false
}

func parseVersion(v string) [3]int {
	var parts [3]int
	v = strings.TrimPrefix(v, "v")
	for i, s := range strings.SplitN(v, ".", 3) {
		if i >= 3 {
			break
		}
		fmt.Sscanf(s, "%d", &parts[i])
	}
	return parts
}

func openReleasesPage() error {
	return exec.Command("open", "https://github.com/SeasX/seasagi/releases").Run()
}

// validateHTTPSURL 强制下载与清单地址使用 HTTPS，避免明文传输被中间人替换。
func validateHTTPSURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("refusing non-https url: %s", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("url has no host: %s", raw)
	}
	return nil
}

func newSecureHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return fmt.Errorf("refusing redirect to non-https url: %s", req.URL.String())
			}
			return nil
		},
	}
}

func fetchChecksumManifest(rawURL string) (map[string]string, error) {
	if err := validateHTTPSURL(rawURL); err != nil {
		return nil, err
	}
	client := newSecureHTTPClient(30 * time.Second)
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checksum manifest returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxChecksumManifestSize))
	if err != nil {
		return nil, err
	}
	manifest := parseChecksumManifest(data)
	if len(manifest) == 0 {
		return nil, fmt.Errorf("checksum manifest is empty or malformed")
	}
	return manifest, nil
}

// parseChecksumManifest 解析常见的 SHA-256 清单格式：
//
//	<hex>  <filename>
//	<hex> *<filename>
//	SHA256 (<filename>) = <hex>
func parseChecksumManifest(data []byte) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "SHA256 (") {
			if idx := strings.Index(line, ") = "); idx > len("SHA256 (") {
				name := strings.TrimSpace(line[len("SHA256 ("):idx])
				sum := strings.ToLower(strings.TrimSpace(line[idx+len(") = "):]))
				if isHexSHA256(sum) && name != "" {
					result[pathBase(name)] = sum
				}
			}
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sum := strings.ToLower(fields[0])
		if !isHexSHA256(sum) {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name != "" {
			result[pathBase(name)] = sum
		}
	}
	return result
}

func isHexSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func lookupChecksum(manifest map[string]string, name string) (string, bool) {
	if sum, ok := manifest[name]; ok {
		return sum, true
	}
	base := pathBase(name)
	if sum, ok := manifest[base]; ok {
		return sum, true
	}
	// 清单里的路径可能与资产 URL 前缀不同，统一按文件名匹配。
	for key, sum := range manifest {
		if pathBase(key) == base {
			return sum, true
		}
	}
	return "", false
}

func pathBase(p string) string {
	return filepath.Base(strings.SplitN(p, "?", 2)[0])
}

// downloadDMG 下载安装包到临时文件（0600），边下边算 SHA-256，校验失败即删除。
func downloadDMG(rawURL, expectedSHA256 string) (string, error) {
	if err := validateHTTPSURL(rawURL); err != nil {
		return "", err
	}
	if !isHexSHA256(strings.ToLower(expectedSHA256)) {
		return "", fmt.Errorf("refusing download without a valid SHA-256 checksum")
	}

	f, err := os.CreateTemp("", "SeasAGI-update-*.dmg")
	if err != nil {
		return "", err
	}
	dmgPath := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(dmgPath)
	}
	if err := f.Chmod(0o600); err != nil {
		cleanup()
		return "", err
	}

	client := newSecureHTTPClient(5 * time.Minute)
	resp, err := client.Get(rawURL)
	if err != nil {
		cleanup()
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		cleanup()
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	hasher := sha256.New()
	limitedReader := io.LimitReader(resp.Body, maxDownloadSize+1)
	written, err := io.Copy(io.MultiWriter(f, hasher), limitedReader)
	if err != nil {
		cleanup()
		return "", err
	}
	if written > maxDownloadSize {
		cleanup()
		return "", fmt.Errorf("download exceeds maximum allowed size")
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(dmgPath)
		return "", err
	}

	got := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(got, expectedSHA256) {
		_ = os.Remove(dmgPath)
		return "", fmt.Errorf("SHA-256 mismatch: expected %s, got %s", expectedSHA256, got)
	}
	return dmgPath, nil
}

func installDMG(dmgPath string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("automatic installation is only supported on macOS")
	}

	mountPoint, err := os.MkdirTemp("", "seasagi-update-mount-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(mountPoint)

	exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()
	if err := exec.Command("hdiutil", "attach", dmgPath, "-mountpoint", mountPoint, "-quiet").Run(); err != nil {
		return fmt.Errorf("mount DMG failed: %w", err)
	}
	defer exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()

	appSource := filepath.Join(mountPoint, "SeasAGI.app")
	if _, err := os.Stat(appSource); os.IsNotExist(err) {
		appSource = ""
		entries, _ := os.ReadDir(mountPoint)
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".app") {
				appSource = filepath.Join(mountPoint, entry.Name())
				break
			}
		}
		if appSource == "" {
			return fmt.Errorf("no .app bundle found in DMG")
		}
	}

	// 安全：安装前校验代码签名与 Gatekeeper 评估，任何一项失败都不落盘。
	if err := verifyAppSignature(appSource); err != nil {
		return err
	}

	homeDir, _ := os.UserHomeDir()
	appDest := filepath.Join(homeDir, "Applications", "SeasAGI.app")
	if err := installAppWithRollback(appSource, appDest); err != nil {
		return err
	}
	return nil
}

// verifyAppSignature 使用 codesign 与 spctl 校验签名/公证状态。
func verifyAppSignature(appPath string) error {
	if out, err := exec.Command("codesign", "--verify", "--deep", "--strict", "--verbose=2", appPath).CombinedOutput(); err != nil {
		return fmt.Errorf("codesign verification failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	if out, err := exec.Command("spctl", "--assess", "--type", "execute", "--verbose=4", appPath).CombinedOutput(); err != nil {
		return fmt.Errorf("Gatekeeper assessment failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// installAppWithRollback 以「备份 → 复制 → 失败回滚」的方式替换应用。
func installAppWithRollback(appSource, appDest string) error {
	backup := ""
	if _, err := os.Stat(appDest); err == nil {
		backup = fmt.Sprintf("%s.bak-%d", appDest, time.Now().UnixNano())
		if err := os.Rename(appDest, backup); err != nil {
			return fmt.Errorf("backup existing app failed: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(appDest), 0o755); err != nil {
		restoreApp(backup, appDest)
		return fmt.Errorf("create applications dir failed: %w", err)
	}
	if err := exec.Command("cp", "-R", appSource, appDest).Run(); err != nil {
		_ = os.RemoveAll(appDest)
		restoreApp(backup, appDest)
		return fmt.Errorf("copy new app failed: %w", err)
	}
	if _, err := os.Stat(appDest); err != nil {
		_ = os.RemoveAll(appDest)
		restoreApp(backup, appDest)
		return fmt.Errorf("installed app not found after copy: %w", err)
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return nil
}

func restoreApp(backup, appDest string) {
	if backup == "" {
		return
	}
	_ = os.RemoveAll(appDest)
	_ = os.Rename(backup, appDest)
}

func StartPeriodicCheck(onUpdateAvailable func(status UpdateStatus)) {
	ticker := time.NewTicker(checkInterval)
	go func() {
		for range ticker.C {
			status, err := checkForUpdate()
			if err != nil || !status.HasUpdate {
				continue
			}
			if onUpdateAvailable != nil {
				onUpdateAvailable(*status)
			}
		}
	}()
}
