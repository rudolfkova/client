package tiles

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"log"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	"client/data"
	"client/internal/world"
)

// AnimFPS кадров в секунду для превью в редакторе (на wire не уходит).
const AnimFPS = 8.0

var (
	animMu       sync.RWMutex
	animMeta     = make(map[string]*animSheet) // base «anim/…» без .png
	editorAnimT  float64                        // секунды, выставляет редактор
	editorAnimTM sync.RWMutex
)

type animSheet struct {
	sheet     *ebiten.Image
	frameCols int
	rows      int
}

// SetEditorAnimTime выставляет глобальную фазу анимации anim/* тайлсетов (секунды монотонно).
// Вызывать каждый кадр из редактора мира и из игрового клиента — иначе кадр анимации залипает на 0.
func SetEditorAnimTime(sec float64) {
	editorAnimTM.Lock()
	editorAnimT = sec
	editorAnimTM.Unlock()
}

func editorAnimSeconds() float64 {
	editorAnimTM.RLock()
	t := editorAnimT
	editorAnimTM.RUnlock()
	return t
}

// IsAnimTilesetBase true, если base — зарегистрированный анимированный лист (anim/…).
func IsAnimTilesetBase(base string) bool {
	animMu.RLock()
	_, ok := animMeta[base]
	animMu.RUnlock()
	return ok
}

// RegisterAnimTilesetFromEmbed грузит PNG: строки = тайлы, столбцы = кадры; клетка world.TileSize.
func RegisterAnimTilesetFromEmbed(embedPath, base string) error {
	animMu.RLock()
	already := animMeta[base] != nil
	animMu.RUnlock()
	if already {
		return nil
	}

	raw, err := data.TileAssets.ReadFile(embedPath)
	if err != nil {
		return err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	cell := world.TileSize
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w%cell != 0 || h%cell != 0 {
		return fmt.Errorf("anim %s: размер %dx%d не кратен клетке %d", base, w, h, cell)
	}
	frameCols, rows := w/cell, h/cell
	if frameCols < 1 || rows < 1 {
		return fmt.Errorf("anim %s: пустой лист", base)
	}
	sheet := ebiten.NewImageFromImage(img)

	animMu.Lock()
	animMeta[base] = &animSheet{sheet: sheet, frameCols: frameCols, rows: rows}
	animMu.Unlock()

	setMu.Lock()
	setTileCount[base] = rows
	setSheetCols[base] = 1
	setMu.Unlock()
	return nil
}

// animImageForTexture если name = base_index (как TextureKey), base — anim-лист, вернуть кадр для текущего времени редактора.
func animImageForTexture(name string) *ebiten.Image {
	base, idx1, ok := ParseIndexedTexture(name)
	if !ok || idx1 < 1 {
		return nil
	}
	row := idx1 - 1
	animMu.RLock()
	meta, ok := animMeta[base]
	animMu.RUnlock()
	if !ok || meta == nil {
		return nil
	}
	if row < 0 || row >= meta.rows {
		return nil
	}
	t := editorAnimSeconds()
	fi := int(t*AnimFPS) % meta.frameCols
	if fi < 0 {
		fi = 0
	}
	x0 := fi * world.TileSize
	y0 := row * world.TileSize
	r := image.Rect(x0, y0, x0+world.TileSize, y0+world.TileSize)
	return meta.sheet.SubImage(r).(*ebiten.Image)
}

func ensureAnimTilesetLoaded(base string) {
	animMu.RLock()
	ok := animMeta[base] != nil
	animMu.RUnlock()
	if ok {
		return
	}
	setMu.Lock()
	if _, failed := setLoadFailed[base]; failed {
		setMu.Unlock()
		return
	}
	setMu.Unlock()

	path := "assets/tileSets/" + base + ".png"
	if err := RegisterAnimTilesetFromEmbed(path, base); err != nil {
		log.Printf("tiles: lazy anim %s: %v", base, err)
		setMu.Lock()
		setLoadFailed[base] = struct{}{}
		setMu.Unlock()
	}
}
