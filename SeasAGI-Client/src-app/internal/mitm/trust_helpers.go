package mitm

import "os"

// writeTempPEM 将 PEM 字节写入权限为 0600 的唯一临时文件，返回文件路径。
// 使用 CreateTemp 避免固定路径被本地其它进程抢占（CL-S11）。
func writeTempPEM(pemData []byte) (string, error) {
	f, err := os.CreateTemp("", "seasagi-mitm-ca-*.pem")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := f.Write(pemData); err != nil {
		f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// removeTempFile 删除临时文件，忽略错误。
func removeTempFile(path string) {
	_ = os.Remove(path)
}
