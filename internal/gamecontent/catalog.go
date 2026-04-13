package gamecontent

import (
	"encoding/json"
	"log"
	"slices"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit/content"
)

// InteractItemIDs — ключи предметов из catalog.json, у которых есть interact (как texture на тайле и item_def_id).
func InteractItemIDs(jsonRaw []byte) []string {
	var c content.Catalog
	if err := json.Unmarshal(jsonRaw, &c); err != nil {
		log.Printf("gamecontent catalog: %v", err)
		return nil
	}
	out := make([]string, 0)
	for id, def := range c.Items {
		if def.Interact == nil {
			continue
		}
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// InteractTextureSet — множество текстур тайлов, по ПКМ на которых отправляется interact.
func InteractTextureSet(jsonRaw []byte) map[string]struct{} {
	ids := InteractItemIDs(jsonRaw)
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}
