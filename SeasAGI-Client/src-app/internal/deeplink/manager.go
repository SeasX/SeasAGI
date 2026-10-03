package deeplink

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
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

	if body := u.Fragment; body != "" {
		params["_fragment"] = body
	}

	return &DeepLinkAction{
		Type:   actionType,
		Params: params,
	}, nil
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

func BuildImportProviderURL(name, providerType, baseURL, apiKey string) string {
	return fmt.Sprintf("seasagi://import-provider?name=%s&type=%s&base_url=%s&api_key=%s",
		url.QueryEscape(name),
		url.QueryEscape(providerType),
		url.QueryEscape(baseURL),
		url.QueryEscape(apiKey),
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
