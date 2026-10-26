package localtoken

import "github.com/SeasAGI/SeasAGI-Client/internal/keychain"

// Store 是本地网关令牌的统一入口。
// 令牌只保存在系统钥匙串（非 macOS 为加密文件），不再写入 SQLite 明文（CL-S12）。
type Store struct{}

// NewStore 保留旧构造签名，便于 main 与 App 逐步迁移。
func NewStore() (*Store, error) {
	return &Store{}, nil
}

func (s *Store) Close() error {
	return nil
}

// GetOrCreate 返回本地网关令牌；不存在时生成并写入钥匙串。
func (s *Store) GetOrCreate() (string, error) {
	return keychain.GetOrCreateAccessToken()
}

// Get 返回已存在的令牌；不存在或读取失败时返回错误。
func (s *Store) Get() (string, error) {
	return keychain.GetAccessToken()
}

// Reset 生成并保存新令牌，使旧令牌立即失效。
func (s *Store) Reset() (string, error) {
	return keychain.ResetAccessToken()
}
