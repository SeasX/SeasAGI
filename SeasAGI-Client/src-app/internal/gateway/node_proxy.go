package gateway

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/protocol"
)

// A 端节点（Inbound Relay）相关常量，需与 relay-gateway 保持一致。
const (
	nodeHeaderTs    = "X-SeasAGI-Node-Ts"
	nodeHeaderSig   = "X-SeasAGI-Node-Sig"
	nodeHeaderID    = "X-SeasAGI-Node-Id"
	nodeHeaderModel = "X-Upstream-Model"
	nodePathPrefix  = "/node"
	nodeSigTTL      = 5 * time.Minute
)

type nodeAuthCtxKey struct{}

var (
	nodeSecretMu    sync.RWMutex
	nodeSecretValue string
)

// SetNodeSecret 设置节点共享密钥；空字符串表示未启用节点模式。
func SetNodeSecret(secret string) {
	nodeSecretMu.Lock()
	nodeSecretValue = secret
	nodeSecretMu.Unlock()
}

// NodeSecretConfigured 报告是否已配置节点密钥。
func NodeSecretConfigured() bool {
	nodeSecretMu.RLock()
	defer nodeSecretMu.RUnlock()
	return nodeSecretValue != ""
}

func currentNodeSecret() string {
	nodeSecretMu.RLock()
	defer nodeSecretMu.RUnlock()
	return nodeSecretValue
}

// NodeProxy 是 A 端节点入口：校验企业 relay 的 HMAC 签名后复用本地转发链路。
// 仅暴露该入口，节点不需要对公网开放 /v1/* 本地接口。
func (s *Service) NodeProxy(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	if !verifyNodeSignature(r, body) {
		writeJSONError(w, http.StatusUnauthorized, "invalid node signature")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, nodePathPrefix)
	if path == "" {
		path = "/v1/chat/completions"
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r = r.WithContext(context.WithValue(r.Context(), nodeAuthCtxKey{}, true))
	s.handleChat(w, r, formatForPath(path))
}

// verifyNodeSignature 校验 HMAC-SHA256(secret, ts + "\n" + path + "\n" + sha256(body)) 与时间窗。
func verifyNodeSignature(r *http.Request, body []byte) bool {
	secret := currentNodeSecret()
	if secret == "" {
		return false
	}
	ts := r.Header.Get(nodeHeaderTs)
	sig := r.Header.Get(nodeHeaderSig)
	if ts == "" || sig == "" {
		return false
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	delta := time.Since(time.Unix(sec, 0))
	if delta > nodeSigTTL || delta < -nodeSigTTL {
		return false
	}

	bodySum := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("\n"))
	mac.Write([]byte(r.URL.Path))
	mac.Write([]byte("\n"))
	mac.Write([]byte(hex.EncodeToString(bodySum[:])))
	expected := hex.EncodeToString(mac.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(expected), []byte(sig)) == 1
}

// formatForPath 依据请求路径推断源协议。
func formatForPath(path string) protocol.Format {
	switch {
	case strings.HasPrefix(path, "/v1/messages"):
		return protocol.FormatAnthropic
	case strings.HasPrefix(path, "/v1/responses"):
		return protocol.FormatOpenAIResponses
	case strings.HasPrefix(path, "/v1beta"):
		return protocol.FormatGemini
	default:
		return protocol.FormatOpenAIChat
	}
}
