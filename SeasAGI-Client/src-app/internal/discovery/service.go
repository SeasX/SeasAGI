package discovery

import (
	"context"
	"fmt"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
	"github.com/SeasAGI/SeasAGI-Client/internal/providers"
)

type DiscoveredModel struct {
	ChannelID    string `json:"channel_id"`
	ModelID      string `json:"model_id"`
	ModelName    string `json:"model_name"`
	DiscoveredAt string `json:"discovered_at"`
}

type Service struct {
	configSvc *config.Service
}

func NewService(configSvc *config.Service) *Service {
	return &Service{configSvc: configSvc}
}

func (s *Service) TestConnectivity(channelID string) (bool, error) {
	channel, ok := s.configSvc.GetChannel(channelID)
	if !ok {
		return false, fmt.Errorf("channel not found")
	}

	cfg, err := providerConfigFromChannel(channel)
	if err != nil {
		return false, err
	}

	adapter := providers.ResolveExecutor(cfg)
	if _, err := adapter.ListModels(context.Background(), cfg); err != nil {
		_ = s.configSvc.UpdateChannelHealth(channelID, "unhealthy")
		return false, err
	}
	_ = s.configSvc.UpdateChannelHealth(channelID, "healthy")
	return true, nil
}

func (s *Service) DiscoverModels(channelID string) ([]DiscoveredModel, error) {
	channel, ok := s.configSvc.GetChannel(channelID)
	if !ok {
		return nil, fmt.Errorf("channel not found")
	}

	cfg, err := providerConfigFromChannel(channel)
	if err != nil {
		return nil, err
	}

	adapter := providers.ResolveExecutor(cfg)
	models, err := adapter.ListModels(context.Background(), cfg)
	if err != nil {
		_ = s.configSvc.UpdateChannelHealth(channelID, "unhealthy")
		return nil, err
	}

	result := make([]DiscoveredModel, 0, len(models))
	modelIDs := make([]string, 0, len(models))
	for _, model := range models {
		result = append(result, DiscoveredModel{
			ChannelID:    channelID,
			ModelID:      model.ID,
			ModelName:    model.ID,
			DiscoveredAt: modelTime(),
		})
		modelIDs = append(modelIDs, model.ID)
	}
	_ = s.configSvc.UpdateChannelModels(channelID, modelIDs)
	_ = s.configSvc.UpdateChannelHealth(channelID, "healthy")
	return result, nil
}

func providerConfigFromChannel(channel config.Channel) (*providers.ProviderConfig, error) {
	apiKey := channel.APIKey
	if apiKey == "" && channel.ChannelType == "custom" {
		var err error
		apiKey, err = keychain.GetChannelKey(channel.ChannelID)
		if err != nil {
			return nil, err
		}
	}

	return &providers.ProviderConfig{
		ChannelType:            channel.ChannelType,
		ProviderType:           channel.ProviderType,
		BaseURL:                channel.BaseURL,
		APIKey:                 apiKey,
		ProviderSpecificConfig: channel.ProviderSpecificConfig,
	}, nil
}

func modelTime() string {
	return time.Now().UTC().Format(time.RFC3339)
}
