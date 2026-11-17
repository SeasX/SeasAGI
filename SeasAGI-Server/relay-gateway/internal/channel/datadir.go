package channel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
)

// dataDirEnvVars 是 relay 持久化目录的配置项，按顺序取第一个非空值。
var dataDirEnvVars = []string{"RELAY_DATA_DIR", "DATA_DIR"}

// legacyDataDirName 是升级前固定使用的 $HOME 下目录名；未配置环境变量时继续沿用，保证兼容。
const legacyDataDirName = ".seasagi"

const dbFileName = "relay-gateway.db"

// ResolveDataDir 决定 relay 的持久化目录：RELAY_DATA_DIR > DATA_DIR > $HOME/.seasagi。
//
// 显式配置了新目录时绝不自动迁移/覆盖/删除旧目录中的数据库；若检测到旧目录仍有数据
// 而新目录为空，只打印告警，提示运维手动迁移。
func ResolveDataDir() (string, error) {
	for _, name := range dataDirEnvVars {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			dir, err := filepath.Abs(v)
			if err != nil {
				return "", fmt.Errorf("invalid %s %q: %w", name, v, err)
			}
			warnIfLegacyDataLeftBehind(dir)
			return dir, nil
		}
	}
	legacy, err := legacyDataDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine data directory (%v); set RELAY_DATA_DIR", err)
	}
	return legacy, nil
}

func legacyDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("empty home directory")
	}
	return filepath.Join(home, legacyDataDirName), nil
}

func warnIfLegacyDataLeftBehind(dir string) {
	legacy, err := legacyDataDir()
	if err != nil || filepath.Clean(legacy) == filepath.Clean(dir) {
		return
	}
	legacyDB := filepath.Join(legacy, dbFileName)
	if _, err := os.Stat(legacyDB); err != nil {
		return
	}
	if _, err := os.Stat(filepath.Join(dir, dbFileName)); err == nil {
		return
	}
	logging.Warningf("relay data dir is %s but an existing database was found at %s; it is NOT migrated automatically — stop the service and copy it manually if you want to keep local channel state", dir, legacyDB)
}
