package gamecontent

import (
	"encoding/json"
	"strings"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit/content"
)

// EditorInstanceArgsDraft — текст для модалки редактора: пример из каталога или дефолт.
func EditorInstanceArgsDraft(catalogRaw []byte, itemID string) string {
	var c content.Catalog
	if err := json.Unmarshal(catalogRaw, &c); err != nil {
		return defaultInstanceArgsDraft()
	}
	def, ok := c.Items[itemID]
	if !ok || def.Interact == nil || len(def.Interact.EditorInstanceArgsExample) == 0 {
		return defaultInstanceArgsDraft()
	}
	b, err := json.MarshalIndent(def.Interact.EditorInstanceArgsExample, "", "  ")
	if err != nil {
		return defaultInstanceArgsDraft()
	}
	return string(b)
}

func defaultInstanceArgsDraft() string {
	// world_spawn_tile на сервере читает только плоские x, y, layer, … — не вложенный target.
	return "{\n  \"x\": 4,\n  \"y\": 4\n}"
}

// ParseAndNormalizeInstanceArgsText проверяет черновик JSON; при успехе возвращает нормализованный raw или nil (пустой объект).
// errMsg непустой при ошибке; при пустом объекте errMsg == "" и raw == nil.
func ParseAndNormalizeInstanceArgsText(s string) (raw json.RawMessage, errMsg string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, "введите JSON-объект или {}"
	}
	var probe any
	if err := json.Unmarshal([]byte(s), &probe); err != nil {
		return nil, "JSON: " + err.Error()
	}
	if _, ok := probe.(map[string]any); !ok {
		return nil, "нужен объект {...}, не массив и не примитив"
	}
	compact, err := json.Marshal(probe)
	if err != nil {
		return nil, err.Error()
	}
	if len(compact) > gamekit.MaxTileInstanceArgsJSONBytes {
		return nil, "объект больше лимита сервера (64 KiB)"
	}
	norm := gamekit.NormalizeTileInstanceArgsJSON(compact)
	if norm == nil {
		return nil, ""
	}
	return norm, ""
}
