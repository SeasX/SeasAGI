//go:build darwin

package keychain

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation

#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <Security/Security.h>

// kc_copy_error 把 OSStatus 转成可读的错误字符串（调用方负责 free）。
static char *kc_copy_error(OSStatus status) {
	CFStringRef msg = SecCopyErrorMessageString(status, NULL);
	char *out = NULL;
	if (msg) {
		CFIndex maxLen = CFStringGetMaximumSizeForEncoding(CFStringGetLength(msg), kCFStringEncodingUTF8) + 1;
		out = (char *)malloc((size_t)maxLen);
		if (out && !CFStringGetCString(msg, out, maxLen, kCFStringEncodingUTF8)) {
			free(out);
			out = NULL;
		}
		CFRelease(msg);
	}
	if (!out) {
		out = (char *)malloc(32);
		if (out) {
			snprintf(out, 32, "OSStatus %d", (int)status);
		}
	}
	return out;
}

static CFMutableDictionaryRef kc_base_query(const char *service, const char *account) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	if (!q) {
		return NULL;
	}
	CFStringRef svc = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
	CFStringRef acc = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	if (svc) {
		CFDictionarySetValue(q, kSecAttrService, svc);
		CFRelease(svc);
	}
	if (acc) {
		CFDictionarySetValue(q, kSecAttrAccount, acc);
		CFRelease(acc);
	}
	return q;
}

// kc_read 返回 0=成功，1=条目不存在，-1=失败。secret 由调用方 free。
static int kc_read(const char *service, const char *account, char **out_secret, char **out_err) {
	*out_secret = NULL;
	*out_err = NULL;
	CFMutableDictionaryRef q = kc_base_query(service, account);
	if (!q) {
		*out_err = kc_copy_error(errSecAllocate);
		return -1;
	}
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching(q, &result);
	CFRelease(q);
	if (status == errSecItemNotFound) {
		return 1;
	}
	if (status != errSecSuccess) {
		*out_err = kc_copy_error(status);
		return -1;
	}
	if (result && CFGetTypeID(result) == CFDataGetTypeID()) {
		CFDataRef data = (CFDataRef)result;
		CFIndex len = CFDataGetLength(data);
		char *buf = (char *)malloc((size_t)len + 1);
		if (!buf) {
			CFRelease(result);
			*out_err = kc_copy_error(errSecAllocate);
			return -1;
		}
		memcpy(buf, CFDataGetBytePtr(data), (size_t)len);
		buf[len] = '\0';
		*out_secret = buf;
	}
	if (result) {
		CFRelease(result);
	}
	return 0;
}

// kc_write 先更新、不存在则新增；任何情况下都不把密钥放进命令行参数。
static int kc_write(const char *service, const char *account, const char *secret, char **out_err) {
	*out_err = NULL;
	CFDataRef data = CFDataCreate(NULL, (const UInt8 *)secret, (CFIndex)strlen(secret));
	if (!data) {
		*out_err = kc_copy_error(errSecAllocate);
		return -1;
	}
	CFMutableDictionaryRef query = kc_base_query(service, account);
	if (!query) {
		CFRelease(data);
		*out_err = kc_copy_error(errSecAllocate);
		return -1;
	}
	CFMutableDictionaryRef attrs = CFDictionaryCreateMutable(NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	if (!attrs) {
		CFRelease(data);
		CFRelease(query);
		*out_err = kc_copy_error(errSecAllocate);
		return -1;
	}
	CFDictionarySetValue(attrs, kSecValueData, data);
	OSStatus status = SecItemUpdate(query, attrs);
	if (status == errSecItemNotFound) {
		CFDictionarySetValue(query, kSecValueData, data);
		CFDictionarySetValue(query, kSecAttrAccessible, kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly);
		status = SecItemAdd(query, NULL);
	}
	CFRelease(attrs);
	CFRelease(query);
	CFRelease(data);
	if (status != errSecSuccess) {
		*out_err = kc_copy_error(status);
		return -1;
	}
	return 0;
}

static int kc_remove(const char *service, const char *account, char **out_err) {
	*out_err = NULL;
	CFMutableDictionaryRef query = kc_base_query(service, account);
	if (!query) {
		*out_err = kc_copy_error(errSecAllocate);
		return -1;
	}
	OSStatus status = SecItemDelete(query);
	CFRelease(query);
	if (status == errSecSuccess || status == errSecItemNotFound) {
		return 0;
	}
	*out_err = kc_copy_error(status);
	return -1;
}
*/
import "C"

import (
	"fmt"
	"strings"
	"unsafe"
)

// macOS：直接调用 Security.framework（SecItem*），密钥只存在于进程内存与系统钥匙串，
// 不再以 `security -w <secret>` 的形式出现在子进程 argv 中（CL-S4）。

func cString(s string) *C.char {
	return C.CString(s)
}

func freeCString(p *C.char) {
	if p != nil {
		C.free(unsafe.Pointer(p))
	}
}

func cErrString(p *C.char) string {
	if p == nil {
		return "unknown keychain error"
	}
	return strings.TrimSpace(C.GoString(p))
}

func platformGet(keyID string) (string, error) {
	cKey := cString(keyID)
	cSvc := cString(serviceName)
	defer freeCString(cKey)
	defer freeCString(cSvc)

	var secret *C.char
	var cErr *C.char
	rc := C.kc_read(cSvc, cKey, &secret, &cErr)
	defer freeCString(secret)
	defer freeCString(cErr)

	switch rc {
	case 1:
		return "", nil
	case 0:
		if secret == nil {
			return "", nil
		}
		return strings.TrimSpace(C.GoString(secret)), nil
	default:
		return "", fmt.Errorf("read keychain entry %s: %s", keyID, cErrString(cErr))
	}
}

func platformSave(keyID, secret string) error {
	cKey := cString(keyID)
	cSvc := cString(serviceName)
	cSecret := cString(secret)
	defer freeCString(cKey)
	defer freeCString(cSvc)
	defer freeCString(cSecret)

	var cErr *C.char
	rc := C.kc_write(cSvc, cKey, cSecret, &cErr)
	defer freeCString(cErr)
	if rc != 0 {
		return fmt.Errorf("save keychain entry %s: %s", keyID, cErrString(cErr))
	}
	return nil
}

func platformDelete(keyID string) error {
	cKey := cString(keyID)
	cSvc := cString(serviceName)
	defer freeCString(cKey)
	defer freeCString(cSvc)

	var cErr *C.char
	rc := C.kc_remove(cSvc, cKey, &cErr)
	defer freeCString(cErr)
	if rc != 0 {
		return fmt.Errorf("delete keychain entry %s: %s", keyID, cErrString(cErr))
	}
	return nil
}
