package gamecontent

import (
	"encoding/json"
	"strings"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit/content"
)

// ItemCanPlaceInInventorySlot reports whether itemDefID may go into slot per embedded catalog.
// Empty itemDefID: true. JSON parse error or empty items map: true (permissive, mirrors server without catalog).
func ItemCanPlaceInInventorySlot(jsonRaw []byte, slot, itemDefID string) bool {
	itemDefID = strings.TrimSpace(itemDefID)
	if itemDefID == "" {
		return true
	}
	var c content.Catalog
	if err := json.Unmarshal(jsonRaw, &c); err != nil {
		return true
	}
	if len(c.Items) == 0 {
		return true
	}
	return content.ItemFitsInventorySlot(&c, itemDefID, slot)
}

// ItemCanPlaceInBackpack — false, если в каталоге у id задано is_storage (в рюкзак нельзя).
func ItemCanPlaceInBackpack(jsonRaw []byte, itemDefID string) bool {
	return ItemCanPlaceInInventorySlot(jsonRaw, gamekit.InvSlotBackpackPref+"0", itemDefID)
}
