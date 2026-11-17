package config

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// 配置 JSON 兼容性契约：用户磁盘上的 config.json 由 AppConfig 的 json 标签决定。
// 配置服务按领域拆分（channels / combos / settings / quota ...）时，
// 任何已发布字段的 json 键都不能被删除或改名，否则旧配置升级后会静默丢失。
//
// 新增字段后更新契约：go test ./internal/config -run JSONContract -update-config-contract
var updateConfigContract = flag.Bool("update-config-contract", false, "rewrite config JSON key contract")

// jsonKeyPaths 递归列出类型的全部 json 键路径（结构体 / 切片 / map 值中的结构体）。
func jsonKeyPaths(t reflect.Type, prefix string, seen map[reflect.Type]bool, out map[string]bool) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || seen[t] {
		return
	}
	seen[t] = true
	defer delete(seen, t)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if f.Anonymous && name == "" {
			jsonKeyPaths(f.Type, prefix, seen, out)
			continue
		}
		if name == "" {
			name = f.Name
		}
		path := prefix + name
		out[path] = true
		jsonKeyPaths(f.Type, path+".", seen, out)
	}
}

func TestAppConfigJSONContract(t *testing.T) {
	keys := map[string]bool{}
	jsonKeyPaths(reflect.TypeOf(AppConfig{}), "", map[reflect.Type]bool{}, keys)
	current := make([]string, 0, len(keys))
	for k := range keys {
		current = append(current, k)
	}
	sort.Strings(current)

	path := filepath.Join("testdata", "appconfig_json_keys.txt")
	if *updateConfigContract {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.Join(current, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing contract %s (run with -update-config-contract): %v", path, err)
	}
	var removed []string
	for _, k := range strings.Fields(string(raw)) {
		if !keys[k] {
			removed = append(removed, k)
		}
	}
	if len(removed) > 0 {
		t.Fatalf("config.json keys removed or renamed (breaks existing user configs): %v", removed)
	}
}

// 一个填充了所有顶层字段的配置经保存/加载往返后不丢失数据。
func TestAppConfigJSONRoundTrip(t *testing.T) {
	var cfg AppConfig
	v := reflect.ValueOf(&cfg).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString("x-" + v.Type().Field(i).Name)
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int, reflect.Int64, reflect.Int32:
			f.SetInt(int64(i + 1))
		case reflect.Float64, reflect.Float32:
			f.SetFloat(float64(i) + 0.5)
		}
	}
	first, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back AppConfig
	if err := json.Unmarshal(first, &back); err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(back)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("round trip changed config JSON\nfirst:  %s\nsecond: %s", first, second)
	}
}
