package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	releaseCheckURL = "https://api.github.com/repos/SeasX/seasagi/releases/latest"
	currentVersion  = "0.2.0"
	checkInterval   = 4 * time.Hour
)

type ReleaseInfo struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type UpdateStatus struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	HasUpdate      bool   `json:"has_update"`
	ReleaseNotes   string `json:"release_notes"`
	DownloadURL    string `json:"download_url"`
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

	dmgPath, err := downloadDMG(status.DownloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	return installDMG(dmgPath)
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
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	hasUpdate := versionGreaterThan(latestVersion, currentVersion)

	downloadURL := ""
	if hasUpdate {
		suffix := fmt.Sprintf("%s-%s.dmg", runtime.GOOS, runtime.GOARCH)
		if runtime.GOOS == "darwin" && (runtime.GOARCH == "arm64" || runtime.GOARCH == "amd64") {
			suffix = "macos.dmg"
		}
		for _, asset := range release.Assets {
			if strings.HasSuffix(asset.Name, suffix) || strings.HasSuffix(asset.Name, ".dmg") {
				downloadURL = asset.BrowserDownloadURL
				break
			}
		}
		if downloadURL == "" {
			downloadURL = release.HTMLURL
		}
	}

	return &UpdateStatus{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
		HasUpdate:      hasUpdate,
		ReleaseNotes:   release.Body,
		DownloadURL:    downloadURL,
	}, nil
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

func downloadDMG(url string) (string, error) {
	homeDir, _ := os.UserHomeDir()
	downloadDir := filepath.Join(homeDir, "Downloads")
	os.MkdirAll(downloadDir, 0755)

	fileName := fmt.Sprintf("SeasAGI-update-%d.dmg", time.Now().Unix())
	dmgPath := filepath.Join(downloadDir, fileName)

	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	f, err := os.Create(dmgPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Limit download size to 1GB to prevent disk fill attacks
	limitedReader := io.LimitReader(resp.Body, 1<<30)
	if _, err := f.ReadFrom(limitedReader); err != nil {
		os.Remove(dmgPath)
		return "", err
	}

	return dmgPath, nil
}

func installDMG(dmgPath string) error {
	mountPoint := "/tmp/seasagi-update"
	exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()

	if err := exec.Command("hdiutil", "attach", dmgPath, "-mountpoint", mountPoint, "-quiet").Run(); err != nil {
		return fmt.Errorf("mount DMG failed: %w", err)
	}
	defer exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()

	appSource := filepath.Join(mountPoint, "SeasAGI.app")
	if _, err := os.Stat(appSource); os.IsNotExist(err) {
		entries, _ := os.ReadDir(mountPoint)
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".app") {
				appSource = filepath.Join(mountPoint, entry.Name())
				break
			}
		}
	}

	homeDir, _ := os.UserHomeDir()
	appDest := filepath.Join(homeDir, "Applications", "SeasAGI.app")

	if err := exec.Command("rm", "-rf", appDest).Run(); err != nil {
		return fmt.Errorf("remove old app failed: %w", err)
	}

	if err := exec.Command("cp", "-R", appSource, appDest).Run(); err != nil {
		return fmt.Errorf("copy new app failed: %w", err)
	}

	return nil
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
