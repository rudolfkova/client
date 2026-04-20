package gamecontent

import (
	"encoding/json"
	"strings"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
)

const (
	doorTriggerCatalogID = "door_trigger"
	treeTriggerCatalogID = "tree_trigger"
)

// IsInvisibleTriggerCatalogID reports catalog items that use invisible.png on the wire and carry item_def_id in instance_args.
func IsInvisibleTriggerCatalogID(id string) bool {
	switch strings.TrimSpace(id) {
	case doorTriggerCatalogID, treeTriggerCatalogID:
		return true
	default:
		return false
	}
}

// CatalogSpawnWireAndInstanceArgs: для невидимых триггеров на wire уходит текстура invisible, в instance_args — item_def_id каталога (если ещё не задан).
func CatalogSpawnWireAndInstanceArgs(catalogItemID string, instanceArgs json.RawMessage) (wireTex string, merged json.RawMessage) {
	wireTex = catalogItemID
	if !IsInvisibleTriggerCatalogID(catalogItemID) {
		return wireTex, instanceArgs
	}
	wireTex = gamekit.InvisibleTileTextureKey
	var m map[string]any
	if len(instanceArgs) > 0 {
		_ = json.Unmarshal(instanceArgs, &m)
	}
	if m == nil {
		m = map[string]any{}
	}
	if _, ok := m["item_def_id"]; !ok {
		m["item_def_id"] = strings.TrimSpace(catalogItemID)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return wireTex, instanceArgs
	}
	return wireTex, gamekit.NormalizeTileInstanceArgsJSON(b)
}
