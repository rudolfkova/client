package gamecontent

import (
	"encoding/json"
	"strings"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit/content"
)

// ItemCanPlaceInBackpack — false, если в каталоге у id задано is_storage (в рюкзак нельзя).
// Нет id в каталоге или ошибка разбора — true (как dev без каталога на сервере).
func ItemCanPlaceInBackpack(jsonRaw []byte, itemDefID string) bool {
	itemDefID = strings.TrimSpace(itemDefID)
	if itemDefID == "" {
		return true
	}
	var c content.Catalog
	if err := json.Unmarshal(jsonRaw, &c); err != nil {
		return true
	}
	def, ok := c.Items[itemDefID]
	if !ok {
		return true
	}
	return !def.IsStorage
}
