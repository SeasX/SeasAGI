package channel

import (
	shared "github.com/SeasAGI/SeasAGI-Server/shared/secure"
)

// relayCipherPrefix 标记 relay 落库的上游凭证密文；无此前缀的非空值视为升级前的历史明文。
const relayCipherPrefix = "relay-enc:v1:"

// relayCredentialBox 是 relay-gateway 的凭证加密配置。
//
// 主密钥优先读取 RELAY_DATA_KEY，未配置时回退 SEASAGI_DATA_KEY；HKDF salt/info
// 与 platform-api 不同，即使两端共用同一主密钥，派生出的 AES 密钥也互不相同。
var relayCredentialBox = shared.Box{
	KeyEnv: []string{"RELAY_DATA_KEY", "SEASAGI_DATA_KEY"},
	Salt:   []byte("seasagi/relay/channel-key/salt/v1"),
	Info:   []byte("seasagi/relay/channel-key/aes-gcm/v1"),
	Prefix: relayCipherPrefix,
}

// credentialCipher 封装凭证落库前加密 / 读库后解密的规则。
type credentialCipher struct {
	box shared.Box
}

func newCredentialCipher() credentialCipher {
	return credentialCipher{box: relayCredentialBox}
}

// available 返回 nil 表示可以加密落库；否则凭证只保留在内存中，绝不以明文写盘。
func (c credentialCipher) available() error {
	return c.box.Check()
}

func (c credentialCipher) seal(plain string) (string, error) {
	return c.box.Encrypt(plain)
}

// unseal 解析库中存储的凭证值：
//   - 空串：无凭证；
//   - 无前缀：升级前的历史明文（legacy=true），调用方负责迁移为密文；
//   - 有前缀：解密，失败时返回错误（调用方据此把渠道标记为凭证不可用）。
func (c credentialCipher) unseal(stored string) (plain string, legacy bool, err error) {
	if stored == "" {
		return "", false, nil
	}
	if !c.box.IsCiphertext(stored) {
		return stored, true, nil
	}
	plain, err = c.box.Decrypt(stored)
	if err != nil {
		return "", false, err
	}
	return plain, false, nil
}

// credentialDecryptError 是对外暴露的错误类别，不包含任何密钥或密文内容。
const credentialDecryptError = "stored credential cannot be decrypted; check RELAY_DATA_KEY / SEASAGI_DATA_KEY"
