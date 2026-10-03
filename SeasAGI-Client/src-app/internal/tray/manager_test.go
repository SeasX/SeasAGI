package tray

import "testing"

func TestShowAndQuitCallbacks(t *testing.T) {
	shown, quit := false, false
	m := NewManager(func(string) {}, func() { shown = true }, func() { quit = true })
	m.Show()
	m.Quit()
	if !shown {
		t.Fatal("expected onShow callback to fire")
	}
	if !quit {
		t.Fatal("expected onQuit callback to fire")
	}
}

func TestNilCallbacksAreSafe(t *testing.T) {
	m := NewManager(nil, nil, nil)
	m.Show()
	m.Quit()
}

func TestGetChannelsReturnsCopy(t *testing.T) {
	m := NewManager(nil, nil, nil)
	m.UpdateChannels([]ChannelInfo{{ID: "a", DisplayName: "A", Enabled: true}})
	got := m.GetChannels()
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("unexpected channels: %+v", got)
	}
	got[0].ID = "mutated"
	if m.GetChannels()[0].ID != "a" {
		t.Fatal("GetChannels must return a copy of the internal slice")
	}
}

func TestUpdateChannelsWhenNotRunningStoresOnly(t *testing.T) {
	m := NewManager(nil, nil, nil)
	// Not running: UpdateChannels must not invoke the platform tray.
	m.UpdateChannels([]ChannelInfo{{ID: "x"}})
	if len(m.GetChannels()) != 1 {
		t.Fatal("expected channels to be stored")
	}
}
