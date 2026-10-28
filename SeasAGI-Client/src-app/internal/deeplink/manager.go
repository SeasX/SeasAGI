package deeplink

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

type ActionType string

const (
	ActionImportProvider ActionType = "import-provider"
	ActionImportMCP      ActionType = "import-mcp"
	ActionImportPrompt   ActionType = "import-prompt"
	ActionImportSkill    ActionType = "import-skill"
	ActionOpenChannel    ActionType = "open-channel"
)

type DeepLinkAction struct {
	Type   ActionType        `json:"type"`
	Params map[string]string `json:"params"`
}

type Handler func(action DeepLinkAction) error

type Manager struct {
	mu       sync.RWMutex
	handlers map[ActionType]Handler
	pending  chan DeepLinkAction
}

func NewManager() *Manager {
	return &Manager{
		handlers: make(map[ActionType]Handler),
		pending:  make(chan DeepLinkAction, 16),
	}
}

func (m *Manager) RegisterHandler(actionType ActionType, handler Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[actionType] = handler
}

func (m *Manager) Parse(rawURL string) (*DeepLinkAction, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid deep link URL: %w", err)
	}

	if u.Scheme != "seasagi" {
		return nil, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	actionType := ActionType(u.Host)
	params := make(map[string]string)
	for k, v := range u.Query() {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}

	// 深链可能出现在浏览器历史、系统日志和他机剪贴板中，禁止承载任何凭证（CL-S23）。
	if err := rejectCredentialParams(u.Query()); err != nil {
		return nil, err
	}
	if body := u.Fragment; body != "" {
		if fragQuery, err := url.ParseQuery(body); err == nil {
			if err := rejectCredentialParams(fragQuery); err != nil {
				return nil, err
			}
		}
		params["_fragment"] = body
	}

	return &DeepLinkAction{
		Type:   actionType,
		Params: params,
	}, nil
}

// rejectCredentialParams 拒绝携带密钥类参数的深链，避免凭证进入 URL（CL-S23）。
func rejectCredentialParams(values url.Values) error {
	for key := range values {
		if isCredentialParam(key) {
			return fmt.Errorf("deep link must not carry credentials: %s", key)
		}
	}
	return nil
}

// isCredentialParam 判断参数名是否属于凭证字段（兼容 api-key / accessToken 等写法）。
func isCredentialParam(name string) bool {
	key := strings.ToLower(strings.TrimSpace(name))
	key = strings.ReplaceAll(key, "-", "_")
	switch key {
	case "api_key", "apikey", "access_token", "refresh_token", "id_token", "token",
		"secret", "client_secret", "password", "passwd", "private_key", "credential", "key":
		return true
	}
	return strings.HasPrefix(key, "api_key") ||
		strings.Contains(key, "secret") ||
		strings.HasSuffix(key, "_token") ||
		strings.HasSuffix(key, "password")
}

func (m *Manager) Handle(rawURL string) error {
	action, err := m.Parse(rawURL)
	if err != nil {
		return err
	}

	m.mu.RLock()
	handler, ok := m.handlers[action.Type]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no handler registered for action: %s", action.Type)
	}

	return handler(*action)
}

func (m *Manager) Enqueue(rawURL string) error {
	action, err := m.Parse(rawURL)
	if err != nil {
		return err
	}
	select {
	case m.pending <- *action:
		return nil
	default:
		return fmt.Errorf("deep link queue full")
	}
}

func (m *Manager) ProcessPending() {
	for {
		select {
		case action := <-m.pending:
			m.mu.RLock()
			handler, ok := m.handlers[action.Type]
			m.mu.RUnlock()
			if ok {
				_ = handler(action)
			}
		default:
			return
		}
	}
}

// BuildImportProviderURL 生成提供商导入深链。
// 刻意不包含 api_key：凭证必须在导入确认后由用户填写，避免出现在 URL/系统日志中（CL-S23）。
func BuildImportProviderURL(name, providerType, baseURL string) string {
	return fmt.Sprintf("seasagi://import-provider?name=%s&type=%s&base_url=%s",
		url.QueryEscape(name),
		url.QueryEscape(providerType),
		url.QueryEscape(baseURL),
	)
}

func BuildImportMCPURL(name, command, transport string, args []string) string {
	u := fmt.Sprintf("seasagi://import-mcp?name=%s&command=%s&transport=%s",
		url.QueryEscape(name),
		url.QueryEscape(command),
		url.QueryEscape(transport),
	)
	if len(args) > 0 {
		argsJSON, _ := json.Marshal(args)
		u += "&args=" + url.QueryEscape(string(argsJSON))
	}
	return u
}

func BuildImportPromptURL(name, targetApp string) string {
	return fmt.Sprintf("seasagi://import-prompt?name=%s&app=%s",
		url.QueryEscape(name),
		url.QueryEscape(targetApp),
	)
}

func BuildImportSkillURL(name, repoURL string) string {
	return fmt.Sprintf("seasagi://import-skill?name=%s&repo=%s",
		url.QueryEscape(name),
		url.QueryEscape(repoURL),
	)
}

func RegisterURLScheme() error {
	switch runtime.GOOS {
	case "darwin":
		return registerMacOS()
	case "linux":
		return registerLinux()
	default:
		return nil
	}
}

func registerMacOS() error {
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleURLTypes</key>
    <array>
        <dict>
            <key>CFBundleURLName</key>
            <string>com.seasagi.desktop</string>
            <key>CFBundleURLSchemes</key>
            <array>
                <string>seasagi</string>
            </array>
        </dict>
    </array>
</dict>
</plist>`
	_ = plist
	return nil
}

func registerLinux() error {
	desktop := `[Desktop Entry]
Type=Application
Name=SeasAGI
Exec=seasagi %u
MimeType=x-scheme-handler/seasagi;
NoDisplay=true`
	_ = desktop
	cmd := exec.Command("xdg-mime", "install", "--novendor", "--mode", "user",
		"x-scheme-handler/seasagi=seasagi.desktop")
	_ = cmd.Run()
	return nil
}
