package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	locales    map[string]map[string]string
	loadOnce   sync.Once
	localesDir string
)

func init() {
	localesDir = "locales"
	if d := os.Getenv("SEASAGI_LOCALES_DIR"); d != "" {
		localesDir = d
	}
}

func load() {
	locales = make(map[string]map[string]string)
	langs := []string{"zh-CN", "en", "ja", "ko"}
	for _, lang := range langs {
		m := make(map[string]string)
		path := filepath.Join(localesDir, lang+".json")
		data, err := os.ReadFile(path)
		if err == nil {
			json.Unmarshal(data, &m)
		}
		locales[lang] = m
	}
}

func T(lang, key string) string {
	loadOnce.Do(load)
	if m, ok := locales[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if m, ok := locales["zh-CN"]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

func GetLangFromRequest(c *gin.Context) string {
	lang := c.Query("lang")
	if lang != "" {
		return lang
	}
	accept := c.GetHeader("Accept-Language")
	if strings.HasPrefix(accept, "en") {
		return "en"
	}
	if strings.HasPrefix(accept, "ja") {
		return "ja"
	}
	if strings.HasPrefix(accept, "ko") {
		return "ko"
	}
	return "zh-CN"
}

func TFromContext(c *gin.Context, key string) string {
	return T(GetLangFromRequest(c), key)
}