package tiles

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	"client/data"
)

// Cell — размер одной клетки в тайлсете (px).
const Cell = 16

var (
	setMu          sync.Mutex
	setTileCount   = make(map[string]int)            // base -> число тайлов после успешной нарезки
	setLoadFailed  = make(map[string]struct{})      // base -> попытка загрузки уже провалилась
)

// ParseIndexedTexture парсит имя вида Beach_Tile_12 (база совпадает с именем PNG без расширения).
func ParseIndexedTexture(name string) (base string, index1 int, ok bool) {
	i := strings.LastIndex(name, "_")
	if i <= 0 {
		return "", 0, false
	}
	base = name[:i]
	if base == "" {
		return "", 0, false
	}
	n, err := strconv.Atoi(name[i+1:])
	if err != nil || n < 1 {
		return "", 0, false
	}
	return base, n, true
}

// TextureKey имя текстуры на wire: слева направо, сверху вниз, с 1 — Beach_Tile_1, Beach_Tile_2, …
func TextureKey(setBase string, index0 int) string {
	return fmt.Sprintf("%s_%d", setBase, index0+1)
}

// EditorTileSets копия списка имён наборов для UI редактора.
func EditorTileSets() []string {
	out := make([]string, len(editorTileSetBases))
	copy(out, editorTileSetBases)
	return out
}

// TileCountInSet сколько тайлов зарегистрировано для набора (после init).
func TileCountInSet(setBase string) int {
	setMu.Lock()
	defer setMu.Unlock()
	return setTileCount[setBase]
}

// ensureTilesetLoaded подгружает assets/tileSets/<base>.png и нарезает клетки (для игрового клиента и новых имён с сервера).
func ensureTilesetLoaded(base string) {
	setMu.Lock()
	if setTileCount[base] > 0 {
		setMu.Unlock()
		return
	}
	if _, failed := setLoadFailed[base]; failed {
		setMu.Unlock()
		return
	}
	setMu.Unlock()

	path := "assets/tileSets/" + base + ".png"
	if _, err := registerTilesetPNG(path, base); err != nil {
		log.Printf("tiles: lazy %s: %v", base, err)
		setMu.Lock()
		setLoadFailed[base] = struct{}{}
		setMu.Unlock()
	}
}

func registerTilesetPNG(embedPath, baseName string) (int, error) {
	raw, err := data.TileAssets.ReadFile(embedPath)
	if err != nil {
		return 0, err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w%Cell != 0 || h%Cell != 0 {
		return 0, fmt.Errorf("размер %dx%d не кратен %d", w, h, Cell)
	}
	cols, rows := w/Cell, h/Cell
	sheet := ebiten.NewImageFromImage(img)

	imgMu.Lock()
	defer imgMu.Unlock()
	idx := 1
	for ry := 0; ry < rows; ry++ {
		for rx := 0; rx < cols; rx++ {
			r := image.Rect(b.Min.X+rx*Cell, b.Min.Y+ry*Cell, b.Min.X+(rx+1)*Cell, b.Min.Y+(ry+1)*Cell)
			sub := sheet.SubImage(r)
			key := fmt.Sprintf("%s_%d", baseName, idx)
			imgByID[key] = ebiten.NewImageFromImage(sub)
			idx++
		}
	}
	n := idx - 1
	setMu.Lock()
	setTileCount[baseName] = n
	setMu.Unlock()
	return n, nil
}
