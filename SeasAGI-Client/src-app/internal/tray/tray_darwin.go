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
	"strings"
	"sync"
	"unsafe"
)

var (
	menuMu       sync.Mutex
	menuByTag    = make(map[int]func())
	activeChanID string
	nextTag      int
	reopenFn     func()
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

// 由 ObjC 的 AppDelegate category（applicationShouldHandleReopen）在主线程调用，
// 语义与托盘"显示窗口"一致：唤出主窗口。
//
//export goTrayReopen
func goTrayReopen() {
	menuMu.Lock()
	fn := reopenFn
	menuMu.Unlock()
	if fn != nil {
		fn()
	}
}

func setReopenHandler(fn func()) {
	menuMu.Lock()
	reopenFn = fn
	menuMu.Unlock()
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

	// ponytail: nextTag 单调递增不归零——异步 clearTrayMenu 生效前点击旧菜单项时，
	// 旧 tag 在新 map 中自然 miss（no-op），不会误触新 handler。
	menuMu.Lock()
	menuByTag = make(map[int]func())
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
		// ponytail: 本地库里可能存有脏数据——无效 UTF-8 字节会让 ObjC 侧
		// stringWithUTF8String: 返回 nil（兜底见 tray_darwin.m 的 nil 防护），
		// 空串则菜单项完全不可辨识，这里统一清洗成可显示的占位内容。
		label = strings.ToValidUTF8(label, "\uFFFD")
		if label == "" {
			label = "(未命名通道)"
		}
		menuMu.Lock()
		active := ch.ID == activeChanID
		menuMu.Unlock()
		checked := 0
		if active {
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
	menuMu.Lock()
	activeChanID = id
	menuMu.Unlock()
}
