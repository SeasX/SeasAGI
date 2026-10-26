//go:build !darwin && !windows

package keychain

// Linux 等平台：优先使用加密文件存储（AES-256-GCM，主密钥 0600）。
// 未引入 Secret Service 依赖时，这是可持久化且不暴露给子进程的最小实现；
// 若后续接入 org.freedesktop.secrets，可在此文件替换为 secret-tool / D-Bus 实现。

func platformGet(keyID string) (string, error) {
	return fileStoreGet(keyID)
}

func platformSave(keyID, secret string) error {
	return fileStoreSave(keyID, secret)
}

func platformDelete(keyID string) error {
	return fileStoreDelete(keyID)
}
