package network

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	modpowrprof         = syscall.NewLazyDLL("powrprof.dll")
	procGetLastWakeTime = modpowrprof.NewProc("GetLastWakeTime")
	modkernel32         = syscall.NewLazyDLL("kernel32.dll")
	procGetTickCount    = modkernel32.NewProc("GetTickCount")
	modiphlpapi         = syscall.NewLazyDLL("iphlpapi.dll")
	procGetAdaptersInfo = modiphlpapi.NewProc("GetAdaptersInfo")
)

func isSystemAwake() bool {
	if procGetLastWakeTime.Find() != nil {
		return true
	}
	var wakeTime uint64
	ret, _, _ := procGetLastWakeTime.Call(uintptr(unsafe.Pointer(&wakeTime)))
	if ret == 0 {
		return true
	}

	var tickCount uint32
	if procGetTickCount.Find() == nil {
		ret2, _, _ := procGetTickCount.Call()
		tickCount = uint32(ret2)
	}

	wakeMs := wakeTime / 10000
	if uint64(tickCount) < wakeMs {
		return false
	}
	return uint64(tickCount)-wakeMs < 60000
}

func captureNetworkID() string {
	if procGetAdaptersInfo.Find() != nil {
		return ""
	}

	buf := make([]byte, 16384)
	bufLen := int32(len(buf))
	ret, _, _ := procGetAdaptersInfo.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&bufLen)),
	)
	if ret != 0 {
		return ""
	}

	id := uint32(0)
	for i := 0; i < len(buf)-4; i++ {
		if buf[i] == 0 && buf[i+1] == 0 && buf[i+2] == 0 && buf[i+3] == 0 {
			break
		}
		id += uint32(buf[i]) * uint32(i+1)
	}
	return fmt.Sprintf("%x", id)
}

func SetAutoLaunch(enabled bool) error {
	advapi32 := syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyEx := advapi32.NewProc("RegOpenKeyExW")
	procRegSetValueEx := advapi32.NewProc("RegSetValueExW")
	procRegDeleteValue := advapi32.NewProc("RegDeleteValueW")
	procRegCloseKey := advapi32.NewProc("RegCloseKey")

	path := `Software\Microsoft\Windows\CurrentVersion\Run`
	var hKey uintptr
	ret, _, _ := procRegOpenKeyEx.Call(
		0x80000001,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(path))),
		0,
		0x02000000|0x0001,
		uintptr(unsafe.Pointer(&hKey)),
	)
	if ret != 0 {
		return fmt.Errorf("failed to open registry key: %d", ret)
	}
	defer procRegCloseKey.Call(hKey)

	if enabled {
		exePath, err := os.Executable()
		if err != nil {
			return err
		}
		utf16Path := syscall.StringToUTF16Ptr(exePath)
		ret, _, _ = procRegSetValueEx.Call(
			hKey,
			uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("SeasAGI"))),
			0,
			1,
			uintptr(unsafe.Pointer(utf16Path)),
			uintptr((len(exePath)+1)*2),
		)
		if ret != 0 {
			return fmt.Errorf("failed to set registry value: %d", ret)
		}
	} else {
		procRegDeleteValue.Call(
			hKey,
			uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("SeasAGI"))),
		)
	}
	return nil
}
