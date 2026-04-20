package gamecontent

import (
	"testing"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
)

func TestItemCanPlaceInInventorySlot_parseErrorPermissive(t *testing.T) {
	if !ItemCanPlaceInInventorySlot([]byte(`{`), gamekit.InvSlotHandMain, "axe") {
		t.Fatal("invalid json should allow swap preview")
	}
}

func TestItemCanPlaceInInventorySlot_rejectsGemInArmor(t *testing.T) {
	raw := []byte(`{"schema_version":1,"items":{"floor_gem":{"id":"floor_gem","pickable":true,"allow_hand":true}}}`)
	if ItemCanPlaceInInventorySlot(raw, gamekit.InvSlotArmor, "floor_gem") {
		t.Fatal("gem without allow_armor must not fit armor slot")
	}
}

func TestItemCanPlaceInInventorySlot_allowsAxeInHand(t *testing.T) {
	raw := []byte(`{"schema_version":1,"items":{"axe":{"id":"axe","pickable":true,"allow_hand":true}}}`)
	if !ItemCanPlaceInInventorySlot(raw, gamekit.InvSlotHandMain, "axe") {
		t.Fatal("axe with allow_hand should fit hand")
	}
}
