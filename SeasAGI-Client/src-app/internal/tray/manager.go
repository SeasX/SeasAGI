package tray

import "sync"

type ChannelInfo struct {
	ID          string
	DisplayName string
	Enabled     bool
}

type Manager struct {
	mu       sync.RWMutex
	channels []ChannelInfo
	onSwitch func(channelID string)
	onShow   func()
	onQuit   func()
	running  bool
}

func NewManager(onSwitch func(channelID string), onShow func(), onQuit func()) *Manager {
	return &Manager{
		onSwitch: onSwitch,
		onShow:   onShow,
		onQuit:   onQuit,
	}
}

func (m *Manager) Run(iconData []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return
	}
	m.running = true
	runPlatformTray(iconData)
	m.rebuild()
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	m.running = false
	stopPlatformTray()
}

func (m *Manager) UpdateChannels(channels []ChannelInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels = channels
	if m.running {
		m.rebuild()
	}
}

func (m *Manager) SetActiveChannel(channelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	setActiveChannel(channelID)
	if m.running {
		m.rebuild()
	}
}

func (m *Manager) GetChannels() []ChannelInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]ChannelInfo, len(m.channels))
	copy(result, m.channels)
	return result
}

func (m *Manager) SwitchTo(channelID string) {
	if m.onSwitch != nil {
		m.onSwitch(channelID)
	}
	m.SetActiveChannel(channelID)
}

func (m *Manager) Show() {
	if m.onShow != nil {
		m.onShow()
	}
}

func (m *Manager) Quit() {
	if m.onQuit != nil {
		m.onQuit()
	}
}

func (m *Manager) rebuild() {
	if !m.running {
		return
	}
	showFn := func() {
		if m.onShow != nil {
			m.onShow()
		}
	}
	quitFn := func() {
		if m.onQuit != nil {
			m.onQuit()
		}
	}
	rebuildPlatformMenu(showFn, quitFn, m.channels, m.onSwitch)
}
