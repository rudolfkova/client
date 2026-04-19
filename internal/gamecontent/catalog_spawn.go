package gamecontent

import (
	"encoding/json"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
)

const doorTriggerCatalogID = "door_trigger"

// CatalogSpawnWireAndInstanceArgs: для door_trigger на wire уходит текстура invisible, в instance_args — item_def_id каталога (если ещё не задан).
func CatalogSpawnWireAndInstanceArgs(catalogItemID string, instanceArgs json.RawMessage) (wireTex string, merged json.RawMessage) {
	wireTex = catalogItemID
	if catalogItemID != doorTriggerCatalogID {
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
		m["item_def_id"] = doorTriggerCatalogID
	}
	b, err := json.Marshal(m)
	if err != nil {
		return wireTex, instanceArgs
	}
	return wireTex, gamekit.NormalizeTileInstanceArgsJSON(b)
}
