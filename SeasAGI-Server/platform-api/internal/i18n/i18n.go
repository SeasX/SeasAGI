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
	locales   map[string]map[string]string
	loadOnce  sync.Once
	localesDir string
)

func init() {
	// Default: look for locales/ relative to the executable or CWD
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

// T returns the translated string for the given language and key.
// Falls back: lang -> zh-CN -> key itself.
func T(lang, key string) string {
	loadOnce.Do(load)
	if m, ok := locales[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	// fallback to zh-CN
	if m, ok := locales["zh-CN"]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

// GetLangFromRequest extracts the language from the request.
// Priority: query param "lang" > Accept-Language header > "zh-CN"
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

// TFromContext translates using the language from the request context.
func TFromContext(c *gin.Context, key string) string {
	return T(GetLangFromRequest(c), key)
}