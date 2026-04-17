package gameclient

import (
	"encoding/json"
	"testing"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
)

func TestItemDefIDFromInstanceArgs(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{
			name: "valid object with item_def_id",
			raw:  json.RawMessage(`{"item_def_id":"tent_folded"}`),
			want: "tent_folded",
		},
		{
			name: "missing item_def_id",
			raw:  json.RawMessage(`{"anchor":true}`),
			want: "",
		},
		{
			name: "invalid json",
			raw:  json.RawMessage(`{"item_def_id"`),
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := itemDefIDFromInstanceArgs(tc.raw)
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestTileItemDefIDFallbackToTexture(t *testing.T) {
	tile := &gamekit.Tile{Texture: "tent_1"}
	if got := tileItemDefID(tile); got != "tent_1" {
		t.Fatalf("want texture fallback, got %q", got)
	}
}

func TestTileItemDefIDPrefersInstanceArgs(t *testing.T) {
	tile := &gamekit.Tile{
		Texture:      "tent_1",
		InstanceArgs: json.RawMessage(`{"item_def_id":"tent_anchor"}`),
	}
	if got := tileItemDefID(tile); got != "tent_anchor" {
		t.Fatalf("want instance args item_def_id, got %q", got)
	}
}
