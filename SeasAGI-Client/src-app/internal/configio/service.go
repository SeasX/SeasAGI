package configio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
)

type ExportData struct {
	Version     string            `json:"version"`
	ExportedAt  string            `json:"exported_at"`
	AppConfig   config.AppConfig  `json:"app_config"`
	Channels    []config.Channel  `json:"channels"`
	MCPServers  []MCPServerExport `json:"mcp_servers,omitempty"`
	PromptPresets []PromptExport  `json:"prompt_presets,omitempty"`
}

type MCPServerExport struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Transport   string            `json:"transport"`
	URL         string            `json:"url,omitempty"`
	Enabled     bool              `json:"enabled"`
	Apps        []string          `json:"apps"`
	Description string            `json:"description,omitempty"`
}

type PromptExport struct {
	Name        string `json:"name"`
	Content     string `json:"content"`
	TargetApp   string `json:"target_app"`
	IsDefault   bool   `json:"is_default"`
	Description string `json:"description,omitempty"`
}

type Service struct {
	configSvc *config.Service
	homeDir   string
}

func NewService(configSvc *config.Service) *Service {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/tmp"
	}
	return &Service{
		configSvc: configSvc,
		homeDir:   home,
	}
}

func (s *Service) Export() ([]byte, error) {
	cfg := s.configSvc.GetConfig()
	channels, err := s.configSvc.ListChannels()
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}

	exportChannels := make([]config.Channel, 0, len(channels))
	for _, ch := range channels {
		ch.APIKey = ""
		exportChannels = append(exportChannels, ch)
	}

	data := ExportData{
		Version:    "1.0",
		ExportedAt: time.Now().Format(time.RFC3339),
		AppConfig:  cfg,
		Channels:   exportChannels,
	}

	mcpData, _ := os.ReadFile(filepath.Join(s.homeDir, ".seasagi", "mcp_servers.json"))
	if len(mcpData) > 0 {
		json.Unmarshal(mcpData, &data.MCPServers)
	}

	promptData, _ := os.ReadFile(filepath.Join(s.homeDir, ".seasagi", "prompt_presets.json"))
	if len(promptData) > 0 {
		json.Unmarshal(promptData, &data.PromptPresets)
	}

	return json.MarshalIndent(data, "", "  ")
}

func (s *Service) Import(jsonData []byte) error {
	var data ExportData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("parse import data: %w", err)
	}

	if data.Version == "" {
		return fmt.Errorf("invalid import data: missing version")
	}

	for _, ch := range data.Channels {
		if ch.ChannelID == "" {
			continue
		}
		if _, err := s.configSvc.SaveCustomChannel(ch); err != nil {
			return fmt.Errorf("save channel %s: %w", ch.ChannelID, err)
		}
	}

	s.configSvc.UpdateRoutingSettings(data.AppConfig.RoutingStrategy, data.AppConfig.StickyChannelUse)
	if data.AppConfig.DefaultModel != "" {
		s.configSvc.SetDefaultModel(data.AppConfig.DefaultModel, data.AppConfig.DefaultChannelID)
	}

	if len(data.MCPServers) > 0 {
		mcpBytes, _ := json.MarshalIndent(data.MCPServers, "", "  ")
		mcpPath := filepath.Join(s.homeDir, ".seasagi", "mcp_servers.json")
		os.MkdirAll(filepath.Dir(mcpPath), 0o755)
		os.WriteFile(mcpPath, mcpBytes, 0o644)
	}

	if len(data.PromptPresets) > 0 {
		promptBytes, _ := json.MarshalIndent(data.PromptPresets, "", "  ")
		promptPath := filepath.Join(s.homeDir, ".seasagi", "prompt_presets.json")
		os.MkdirAll(filepath.Dir(promptPath), 0o755)
		os.WriteFile(promptPath, promptBytes, 0o644)
	}

	return nil
}
