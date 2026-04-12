package characterweb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListAnimSpriteIDs(t *testing.T) {
	root := t.TempDir()
	anim := filepath.Join(root, "anim", "TestSheet", "TestSheet.png")
	if err := os.MkdirAll(filepath.Dir(anim), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(anim, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ListAnimSpriteIDs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "TestSheet" {
		t.Fatalf("got %v", got)
	}
}
