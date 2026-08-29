package mitm

import (
	"os"
	"path/filepath"
)

// writeTempPEM 将 PEM 字节写入临时文件，返回文件路径。
func writeTempPEM(pemData []byte) (string, error) {
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, "seasagi-mitm-ca.pem")
	if err := os.WriteFile(tmpFile, pemData, 0o644); err != nil {
		return "", err
	}
	return tmpFile, nil
}

// removeTempFile 删除临时文件，忽略错误。
func removeTempFile(path string) {
	_ = os.Remove(path)
}
