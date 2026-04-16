package gamecontent

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit/content"
)

// IsCatalogItemID — есть ли id в catalog.items.
func IsCatalogItemID(jsonRaw []byte, id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	var c content.Catalog
	if err := json.Unmarshal(jsonRaw, &c); err != nil {
		return false
	}
	_, ok := c.Items[id]
	return ok
}

// PickableItemIDs — id предметов с pickable: true.
func PickableItemIDs(jsonRaw []byte) []string {
	var c content.Catalog
	if err := json.Unmarshal(jsonRaw, &c); err != nil {
		return nil
	}
	out := make([]string, 0)
	for id, def := range c.Items {
		if !def.Pickable {
			continue
		}
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// CatalogFloorPaletteIDs — id для вкладки «Предметы» редактора: interact ∪ pickable (без дубликатов).
func CatalogFloorPaletteIDs(jsonRaw []byte) []string {
	a := InteractItemIDs(jsonRaw)
	b := PickableItemIDs(jsonRaw)
	seen := make(map[string]struct{}, len(a)+len(b))
	for _, id := range a {
		seen[id] = struct{}{}
	}
	for _, id := range b {
		seen[id] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// PickableTextureSet — множество id с pickable: true (как texture тайла на полу).
func PickableTextureSet(jsonRaw []byte) map[string]struct{} {
	ids := PickableItemIDs(jsonRaw)
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}
