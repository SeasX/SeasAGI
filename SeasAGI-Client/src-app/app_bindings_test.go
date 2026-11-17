package main

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Wails 绑定兼容性：前端通过 frontend/wailsjs/go/main/App.d.ts 调用 App 的导出方法。
// App 拆分为多个 app_*.go 门面后，必须保证：
//  1. App.d.ts 中声明的每个方法在 *App 上仍然存在且参数个数一致（否则前端调用在运行时失败）；
//  2. *App 的每个导出方法都已出现在 App.d.ts 中（否则说明绑定文件过期，需要 wails generate）。
var (
	bindingDecl  = regexp.MustCompile(`(?m)^export function (\w+)\((.*)\):Promise<`)
	bindingParam = regexp.MustCompile(`\barg\d+:`)
)

func TestWailsBindingsMatchAppMethods(t *testing.T) {
	src, err := os.ReadFile("frontend/wailsjs/go/main/App.d.ts")
	if err != nil {
		t.Fatalf("read bindings: %v", err)
	}
	declared := map[string]int{}
	for _, m := range bindingDecl.FindAllStringSubmatch(string(src), -1) {
		// Wails 生成的形参统一命名为 argN，按其个数计数（类型里的逗号不影响）。
		declared[m[1]] = len(bindingParam.FindAllString(m[2], -1))
	}
	if total := strings.Count(string(src), "\nexport function "); len(declared) == 0 || len(declared) != total {
		t.Fatalf("parsed %d of %d bindings from App.d.ts", len(declared), total)
	}

	appType := reflect.TypeOf(&App{})
	var missing, mismatched, undeclared []string
	for name, params := range declared {
		method, ok := appType.MethodByName(name)
		if !ok {
			missing = append(missing, name)
			continue
		}
		// method.Type 的第 0 个入参是接收者。
		if got := method.Type.NumIn() - 1; got != params {
			mismatched = append(mismatched, name)
		}
	}
	for i := 0; i < appType.NumMethod(); i++ {
		if name := appType.Method(i).Name; !hasKey(declared, name) {
			undeclared = append(undeclared, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(mismatched)
	sort.Strings(undeclared)
	if len(missing) > 0 {
		t.Errorf("bindings declared in App.d.ts but missing on *App: %v", missing)
	}
	if len(mismatched) > 0 {
		t.Errorf("binding parameter count differs from *App method: %v", mismatched)
	}
	if len(undeclared) > 0 {
		t.Errorf("*App exports methods absent from App.d.ts (regenerate bindings): %v", undeclared)
	}
}

func hasKey(m map[string]int, key string) bool {
	_, ok := m[key]
	return ok
}
