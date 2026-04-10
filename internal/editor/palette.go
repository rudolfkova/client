package editor

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	textv2 "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"client/internal/tiles"
	"client/internal/ui"
	"client/internal/world"
)

const (
	paletteW = 280

	pad          = 8
	previewSize  = 64
	lineH        = 14
	blocksH      = 20
	tabH         = 22
	setRowH      = 22
	gridTopFixed = pad + previewSize + 4 + lineH + 4 + blocksH + 4 + tabH + 4 + setRowH + 6

	thumbSize = 36
	thumbGap  = 4
	rowH      = 44 // одиночные тайлы, одна колонка
)

func (a *App) paletteX() int {
	if a.winW <= paletteW {
		return 0
	}
	return a.winW - paletteW
}

func (a *App) gridBottom() int {
	return a.winH - pad
}

func (a *App) thumbStep() int {
	return thumbSize + thumbGap
}

func (a *App) gridCols() int {
	inner := paletteW - 2*pad
	step := a.thumbStep()
	if step <= 0 {
		return 1
	}
	c := inner / step
	if c < 1 {
		return 1
	}
	return c
}

func (a *App) maxPaletteScroll() int {
	if a.pickTilesets {
		n := a.tileCount()
		if n <= 0 {
			return 0
		}
		cols := a.gridCols()
		rows := (n + cols - 1) / cols
		h := rows*a.thumbStep() - thumbGap
		avail := a.gridBottom() - gridTopFixed
		if h <= avail {
			return 0
		}
		return h - avail
	}
	keys := tiles.EditorSingleTextureKeys()
	h := len(keys) * rowH
	avail := a.gridBottom() - gridTopFixed
	if h <= avail {
		return 0
	}
	return h - avail
}

func (a *App) clampScroll() {
	m := a.maxPaletteScroll()
	if a.paletteScroll < 0 {
		a.paletteScroll = 0
	}
	if a.paletteScroll > m {
		a.paletteScroll = m
	}
}

func inRect(mx, my, x, y, w, h int) bool {
	return mx >= x && mx < x+w && my >= y && my < y+h
}

func (a *App) inPalette(mx, my int) bool {
	return mx >= a.paletteX() && mx < a.winW && my >= 0 && my < a.winH
}

func (a *App) inGridArea(mx, my int) bool {
	if !a.inPalette(mx, my) {
		return false
	}
	return my >= gridTopFixed && my < a.gridBottom()
}

// handlePaletteScroll вызывать при ненулевом колесе; возвращает true, если прокрутка ушла в палитру.
func (a *App) handlePaletteScroll(mx, my int, wheelY float64) bool {
	if wheelY == 0 || !a.inGridArea(mx, my) {
		return false
	}
	delta := int(math.Round(wheelY * 24))
	a.paletteScroll += delta
	a.clampScroll()
	return true
}

// handlePaletteClick обрабатывает ЛКМ по палитре; true — клик поглощён.
func (a *App) handlePaletteClick(mx, my int) bool {
	if !a.inPalette(mx, my) {
		return false
	}
	px := a.paletteX()

	// Коллизия
	yBlocks := pad + previewSize + 4 + lineH + 4
	if inRect(mx, my, px+pad, yBlocks, paletteW-2*pad, blocksH) {
		a.blocks = !a.blocks
		return true
	}

	// Вкладки
	yTabs := yBlocks + blocksH + 4
	half := (paletteW - 2*pad) / 2
	if inRect(mx, my, px+pad, yTabs, half, tabH) {
		a.pickTilesets = true
		a.clampTileIdx()
		a.paletteScroll = 0
		return true
	}
	if inRect(mx, my, px+pad+half, yTabs, paletteW-2*pad-half, tabH) {
		a.pickTilesets = false
		a.clampSingleIdx()
		a.paletteScroll = 0
		return true
	}

	// Смена набора (только тайлсеты)
	ySet := yTabs + tabH + 4
	if a.pickTilesets {
		btnW := 32
		if inRect(mx, my, px+pad, ySet, btnW, setRowH) {
			a.nextSet(-1)
			a.paletteScroll = 0
			return true
		}
		if inRect(mx, my, px+paletteW-pad-btnW, ySet, btnW, setRowH) {
			a.nextSet(1)
			a.paletteScroll = 0
			return true
		}
	}

	// Сетка / список
	if my < gridTopFixed || my >= a.gridBottom() {
		return true // клик по палитре, но не по полю
	}

	if a.pickTilesets {
		n := a.tileCount()
		if n <= 0 {
			return true
		}
		cols := a.gridCols()
		step := a.thumbStep()
		relX := mx - (px + pad)
		relY := my - gridTopFixed + a.paletteScroll
		if relX < 0 || relY < 0 {
			return true
		}
		col := relX / step
		row := relY / step
		if col < 0 || col >= cols {
			return true
		}
		i := row*cols + col
		if i >= 0 && i < n {
			a.tileIdx = i
		}
		return true
	}

	keys := tiles.EditorSingleTextureKeys()
	if len(keys) == 0 {
		return true
	}
	relY := my - gridTopFixed + a.paletteScroll
	if relY < 0 {
		return true
	}
	i := relY / rowH
	if i >= 0 && i < len(keys) {
		a.singleIdx = i
	}
	return true
}

func (a *App) drawPalette(dst *ebiten.Image) {
	px := float32(a.paletteX())
	pw := float32(paletteW)
	h := float32(a.winH)
	vector.DrawFilledRect(dst, px, 0, pw, h, color.RGBA{0x1e, 0x1e, 0x26, 0xff}, false)
	vector.StrokeLine(dst, px, 0, px, h, 1.5, color.RGBA{0x55, 0x55, 0x66, 0xff}, false)

	face := &textv2.GoTextFace{Source: ui.FontSource(), Size: 12}
	small := &textv2.GoTextFace{Source: ui.FontSource(), Size: 11}

	tex := a.texture()
	prevImg := tiles.ImageForTexture(tex)
	pvx := px + float32(paletteW/2-previewSize/2)
	pvy := float32(pad)
	vector.DrawFilledRect(dst, pvx-2, pvy-2, float32(previewSize)+4, float32(previewSize)+4, color.RGBA{0x30, 0x30, 0x3a, 0xff}, false)
	if prevImg != nil {
		w, h0 := prevImg.Bounds().Dx(), prevImg.Bounds().Dy()
		op := &ebiten.DrawImageOptions{}
		if w > 0 && h0 > 0 {
			op.GeoM.Scale(float64(previewSize)/float64(w), float64(previewSize)/float64(h0))
		}
		op.GeoM.Translate(float64(pvx), float64(pvy))
		dst.DrawImage(prevImg, op)
	} else {
		vector.DrawFilledRect(dst, pvx, pvy, float32(previewSize), float32(previewSize), color.RGBA{0x40, 0x40, 0x50, 0xff}, false)
	}

	yKey := float32(pad + previewSize + 4)
	opts := &textv2.DrawOptions{}
	opts.ColorScale.ScaleWithColor(color.RGBA{0xe8, 0xe8, 0xec, 0xff})
	opts.GeoM.Translate(float64(px+pad), float64(yKey))
	textv2.Draw(dst, tex, small, opts)

	yBlocks := float32(pad + previewSize + 4 + lineH + 4)
	btxt := "Коллизия: нет"
	if a.blocks {
		btxt = "Коллизия: да"
	}
	vector.DrawFilledRect(dst, px+pad, yBlocks, float32(paletteW-2*pad), float32(blocksH), color.RGBA{0x2a, 0x2a, 0x34, 0xff}, false)
	opts = &textv2.DrawOptions{}
	opts.ColorScale.ScaleWithColor(color.RGBA{0xcc, 0xcc, 0xd8, 0xff})
	opts.GeoM.Translate(float64(px+pad+6), float64(yBlocks+4))
	textv2.Draw(dst, btxt, face, opts)

	yTabs := yBlocks + float32(blocksH+4)
	half := float32(paletteW-2*pad) / 2
	tabHi := color.RGBA{0x38, 0x38, 0x48, 0xff}
	tabLo := color.RGBA{0x24, 0x24, 0x2e, 0xff}
	leftCol, rightCol := tabLo, tabLo
	if a.pickTilesets {
		leftCol = tabHi
	} else {
		rightCol = tabHi
	}
	vector.DrawFilledRect(dst, px+pad, yTabs, half, float32(tabH), leftCol, false)
	vector.DrawFilledRect(dst, px+pad+half, yTabs, float32(paletteW-2*pad)-half, float32(tabH), rightCol, false)
	opts = &textv2.DrawOptions{}
	opts.ColorScale.ScaleWithColor(color.RGBA{0xee, 0xee, 0xf0, 0xff})
	opts.GeoM.Translate(float64(px+pad+8), float64(yTabs+4))
	textv2.Draw(dst, "Наборы", face, opts)
	opts.GeoM.Reset()
	opts.GeoM.Translate(float64(px+pad+half+8), float64(yTabs+4))
	textv2.Draw(dst, "Отдельные", face, opts)

	ySet := yTabs + float32(tabH+4)
	if a.pickTilesets {
		setName := a.currentSet()
		if len(setName) > 22 {
			setName = setName[:19] + "…"
		}
		btnW := float32(32)
		vector.DrawFilledRect(dst, px+pad, ySet, btnW, float32(setRowH), color.RGBA{0x3a, 0x3a, 0x48, 0xff}, false)
		vector.DrawFilledRect(dst, px+float32(paletteW-pad)-btnW, ySet, btnW, float32(setRowH), color.RGBA{0x3a, 0x3a, 0x48, 0xff}, false)
		opts = &textv2.DrawOptions{}
		opts.ColorScale.ScaleWithColor(color.RGBA{0xff, 0xff, 0xff, 0xff})
		opts.GeoM.Translate(float64(px+pad+9), float64(ySet+4))
		textv2.Draw(dst, "◀", face, opts)
		opts.GeoM.Reset()
		opts.GeoM.Translate(float64(px+float32(paletteW-pad)-23), float64(ySet+4))
		textv2.Draw(dst, "▶", face, opts)
		opts = &textv2.DrawOptions{}
		opts.PrimaryAlign = textv2.AlignCenter
		opts.GeoM.Translate(float64(px+float32(paletteW)/2), float64(ySet+4))
		opts.ColorScale.ScaleWithColor(color.RGBA{0xd0, 0xd0, 0xd8, 0xff})
		textv2.Draw(dst, setName, small, opts)
	} else {
		opts = &textv2.DrawOptions{}
		opts.ColorScale.ScaleWithColor(color.RGBA{0x88, 0x88, 0x98, 0xff})
		opts.GeoM.Translate(float64(px+pad), float64(ySet+4))
		textv2.Draw(dst, "grass · water · path", small, opts)
	}

	// Сетка миниатюр
	gy0 := float32(gridTopFixed)
	gh := float32(a.gridBottom() - gridTopFixed)
	vector.DrawFilledRect(dst, px+pad, gy0, float32(paletteW-2*pad), gh, color.RGBA{0x18, 0x18, 0x1e, 0xff}, false)

	if a.pickTilesets {
		n := a.tileCount()
		cols := a.gridCols()
		step := float32(a.thumbStep())
		base := a.currentSet()
		for i := 0; i < n; i++ {
			key := tiles.TextureKey(base, i)
			img := tiles.ImageForTexture(key)
			col := i % cols
			row := i / cols
			cx := px + float32(pad) + float32(col)*step
			cy := gy0 + float32(row)*step - float32(a.paletteScroll)
			if cy+float32(thumbSize) < gy0 || cy > gy0+gh {
				continue
			}
			vector.DrawFilledRect(dst, cx, cy, float32(thumbSize), float32(thumbSize), color.RGBA{0x26, 0x26, 0x30, 0xff}, false)
			if img != nil {
				w, h0 := img.Bounds().Dx(), img.Bounds().Dy()
				op := &ebiten.DrawImageOptions{}
				if w > 0 && h0 > 0 {
					op.GeoM.Scale(float64(thumbSize)/float64(w), float64(thumbSize)/float64(h0))
				}
				op.GeoM.Translate(float64(cx), float64(cy))
				dst.DrawImage(img, op)
			}
			if i == a.tileIdx {
				vector.StrokeRect(dst, cx-1, cy-1, float32(thumbSize)+2, float32(thumbSize)+2, 2, color.RGBA{0xff, 0xcc, 0x33, 0xff}, false)
			}
		}
	} else {
		keys := tiles.EditorSingleTextureKeys()
		for i, key := range keys {
			cy := gy0 + float32(i*rowH) - float32(a.paletteScroll)
			if cy+float32(rowH-4) < gy0 || cy > gy0+gh {
				continue
			}
			cx := px + float32(pad)
			tsz := float32(thumbSize)
			vector.DrawFilledRect(dst, cx, cy+4, tsz, tsz, color.RGBA{0x26, 0x26, 0x30, 0xff}, false)
			img := tiles.ImageForTexture(key)
			if img != nil {
				w, h0 := img.Bounds().Dx(), img.Bounds().Dy()
				op := &ebiten.DrawImageOptions{}
				if w > 0 && h0 > 0 {
					op.GeoM.Scale(float64(tsz)/float64(w), float64(tsz)/float64(h0))
				}
				op.GeoM.Translate(float64(cx), float64(cy+4))
				dst.DrawImage(img, op)
			}
			opts = &textv2.DrawOptions{}
			opts.ColorScale.ScaleWithColor(color.RGBA{0xcc, 0xcc, 0xd5, 0xff})
			opts.GeoM.Translate(float64(cx+tsz+8), float64(cy+10))
			textv2.Draw(dst, key, face, opts)
			if i == a.singleIdx {
				rw := float32(paletteW - 2*pad + 2)
				vector.StrokeRect(dst, cx-1, cy+3, rw, float32(thumbSize)+2, 2, color.RGBA{0xff, 0xcc, 0x33, 0xff}, false)
			}
		}
	}
}

func (a *App) mapTileFromCursor(mx, my int) (tx, ty int, ok bool) {
	if mx >= a.paletteX() {
		return 0, 0, false
	}
	return world.TileFromScreen(mx, my)
}
