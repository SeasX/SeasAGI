//go:build darwin

package tray

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <stdlib.h>

void createTray(void *iconData, int iconLen);
void addTrayItem(const char *title, int tag, int isSeparator, int isChecked);
void clearTrayMenu();
void destroyTray();
*/
import "C"
import (
	"sync"
	"unsafe"
)

var (
	menuMu       sync.Mutex
	menuByTag    = make(map[int]func())
	activeChanID string
	nextTag      int
)

//export goTrayMenuClick
func goTrayMenuClick(tag C.int) {
	menuMu.Lock()
	fn, ok := menuByTag[int(tag)]
	menuMu.Unlock()
	if ok && fn != nil {
		fn()
	}
}

func nextMenuTag(fn func()) int {
	menuMu.Lock()
	defer menuMu.Unlock()
	nextTag++
	menuByTag[nextTag] = fn
	return nextTag
}

func runPlatformTray(icon []byte) {
	if len(icon) == 0 {
		C.createTray(nil, 0)
	} else {
		C.createTray(unsafe.Pointer(&icon[0]), C.int(len(icon)))
	}
}

func stopPlatformTray() {
	C.destroyTray()
}

func rebuildPlatformMenu(showFn, quitFn func(), channels []ChannelInfo, onSwitch func(string)) {
	C.clearTrayMenu()

	menuMu.Lock()
	menuByTag = make(map[int]func())
	nextTag = 0
	menuMu.Unlock()

	{
		tag := nextMenuTag(showFn)
		cTitle := C.CString("显示窗口")
		C.addTrayItem(cTitle, C.int(tag), 0, 0)
		C.free(unsafe.Pointer(cTitle))
	}

	{
		C.addTrayItem(nil, 0, 1, 0)
	}

	for _, ch := range channels {
		ch := ch
		label := ch.DisplayName
		if !ch.Enabled {
			label = label + " (已禁用)"
		}
		checked := 0
		if ch.ID == activeChanID {
			checked = 1
		}
		tag := nextMenuTag(func() {
			if onSwitch != nil {
				onSwitch(ch.ID)
			}
			setActiveChannel(ch.ID)
		})
		cTitle := C.CString(label)
		C.addTrayItem(cTitle, C.int(tag), 0, C.int(checked))
		C.free(unsafe.Pointer(cTitle))
	}

	{
		C.addTrayItem(nil, 0, 1, 0)
	}

	{
		tag := nextMenuTag(func() {
			if quitFn != nil {
				quitFn()
			}
		})
		cTitle := C.CString("退出")
		C.addTrayItem(cTitle, C.int(tag), 0, 0)
		C.free(unsafe.Pointer(cTitle))
	}
}

func setActiveChannel(id string) {
	activeChanID = id
}

func getActiveChannel() string {
	return activeChanID
}
