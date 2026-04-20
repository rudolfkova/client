package editor

import (
	"fmt"
	"image/color"
	"math"

	"client/internal/gamecontent"
	"client/internal/tiles"
	"client/internal/ui"
	"client/internal/world"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	textv2 "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
)

const (
	paletteMinW      = 280
	paletteMinMapW   = 420 // минимум ширины области карты слева от палитры
	paletteSplitterW = 6   // зона захвата левого края палитры

	pad         = 8
	previewSize = 64
	lineH       = 14
	blocksH     = 20
	tabH        = 22
	setRowH     = 22
	metaRowH    = 22 // слой и поворот
	thumbSize   = 36
	thumbGap    = 4
	rowH        = 44 // одиночные тайлы, одна колонка
)

func paletteYLayerRow() int { return pad + previewSize + 4 }
func paletteYRotRow() int   { return paletteYLayerRow() + metaRowH + 4 }
func paletteYKey() int      { return paletteYRotRow() + metaRowH + 4 }
func paletteYBlocks() int   { return paletteYKey() + lineH + 4 }
func paletteYTabs() int     { return paletteYBlocks() + blocksH + 4 }
func paletteYSetRow() int   { return paletteYTabs() + tabH + 4 }
func paletteGridTop() int   { return paletteYSetRow() + setRowH + 6 }

func (a *App) paletteX() int {
	pw := a.paletteWidth
	if pw < paletteMinW {
		pw = paletteMinW
	}
	if a.winW <= pw {
		return 0
	}
	return a.winW - pw
}

func (a *App) maxPaletteWidth() int {
	w := a.winW - paletteMinMapW
	if w < paletteMinW {
		return paletteMinW
	}
	return w
}

// clampPaletteWidth держит ширину в [paletteMinW, maxPaletteWidth] (окно, минимум карты слева).
func (a *App) clampPaletteWidth() {
	maxW := a.maxPaletteWidth()
	if a.paletteWidth < paletteMinW {
		a.paletteWidth = paletteMinW
	}
	if a.paletteWidth > maxW {
		a.paletteWidth = maxW
	}
}

func (a *App) inPaletteSplitter(mx, my int) bool {
	px := a.paletteX()
	return mx >= px && mx < px+paletteSplitterW && my >= 0 && my < a.winH
}

func (a *App) gridBottom() int {
	return a.winH - pad
}

func (a *App) thumbStep() int {
	return thumbSize + thumbGap
}

func (a *App) gridCols() int {
	inner := a.paletteWidth - 2*pad
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

// tilesetPaletteCols колонок в сетке = как на PNG листа; иначе запасной вариант по ширине панели.
func (a *App) tilesetPaletteCols() int {
	c := tiles.TilesetSheetCols(a.currentSet())
	if c > 0 {
		return c
	}
	return a.gridCols()
}

func (a *App) gridInnerW() int {
	return a.paletteWidth - 2*pad
}

func (a *App) maxPaletteScrollX() int {
	if !a.pickTilesets {
		return 0
	}
	cols := a.tilesetPaletteCols()
	if cols <= 0 {
		return 0
	}
	step := a.thumbStep()
	contentW := cols*step - thumbGap
	inner := a.gridInnerW()
	if contentW <= inner {
		return 0
	}
	return contentW - inner
}

func (a *App) maxPaletteScroll() int {
	if a.pickTilesets {
		n := a.tileCount()
		if n <= 0 {
			return 0
		}
		cols := a.tilesetPaletteCols()
		rows := (n + cols - 1) / cols
		h := rows*a.thumbStep() - thumbGap
		avail := a.gridBottom() - paletteGridTop()
		if h <= avail {
			return 0
		}
		return h - avail
	}
	if a.pickCatalog {
		n := len(a.catalogInteractIDs)
		h := n * rowH
		avail := a.gridBottom() - paletteGridTop()
		if h <= avail {
			return 0
		}
		return h - avail
	}
	keys := tiles.EditorSingleTextureKeys()
	h := len(keys) * rowH
	avail := a.gridBottom() - paletteGridTop()
	if h <= avail {
		return 0
	}
	return h - avail
}

func (a *App) clampScroll() {
	my := a.maxPaletteScroll()
	if a.paletteScroll < 0 {
		a.paletteScroll = 0
	}
	if a.paletteScroll > my {
		a.paletteScroll = my
	}
	mx := a.maxPaletteScrollX()
	if a.paletteScrollX < 0 {
		a.paletteScrollX = 0
	}
	if a.paletteScrollX > mx {
		a.paletteScrollX = mx
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
	return my >= paletteGridTop() && my < a.gridBottom()
}

// handlePaletteScroll колесо над сеткой: вертикаль; горизонталь — wheelX (тачпад) или Shift+колёсико.
func (a *App) handlePaletteScroll(mx, my int, wheelX, wheelY float64) bool {
	if (wheelX == 0 && wheelY == 0) || !a.inGridArea(mx, my) {
		return false
	}
	shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
	dx, dy := wheelX, wheelY
	if shift && wheelY != 0 {
		dx += wheelY
		dy = 0
	}
	if dy != 0 {
		a.paletteScroll += int(math.Round(dy * 24))
	}
	if dx != 0 {
		a.paletteScrollX += int(math.Round(dx * 24))
	}
	a.clampScroll()
	return true
}

// handlePaletteClick обрабатывает ЛКМ по палитре; true — клик поглощён.
func (a *App) handlePaletteClick(mx, my int) bool {
	if !a.inPalette(mx, my) {
		return false
	}
	if a.inPaletteSplitter(mx, my) {
		return true
	}
	px := a.paletteX()
	pw := a.paletteWidth
	btnW := 32

	// Слой
	yl := paletteYLayerRow()
	if inRect(mx, my, px+pad, yl, btnW, metaRowH) {
		a.decLayer()
		return true
	}
	if inRect(mx, my, px+pw-pad-btnW, yl, btnW, metaRowH) {
		a.incLayer()
		return true
	}

	// Поворот (четверти по часовой)
	yr := paletteYRotRow()
	if inRect(mx, my, px+pad, yr, btnW, metaRowH) {
		a.stepRotation(-1)
		return true
	}
	if inRect(mx, my, px+pw-pad-btnW, yr, btnW, metaRowH) {
		a.stepRotation(1)
		return true
	}

	// Коллизия
	yBlocks := paletteYBlocks()
	if inRect(mx, my, px+pad, yBlocks, pw-2*pad, blocksH) {
		a.blocks = !a.blocks
		return true
	}

	// Вкладки (три колонки)
	yTabs := paletteYTabs()
	tabW := (pw - 2*pad) / 3
	tabRest := pw - 2*pad - 2*tabW
	if inRect(mx, my, px+pad, yTabs, tabW, tabH) {
		a.pickTilesets = true
		a.pickCatalog = false
		a.clampTileIdx()
		a.setTileBrush1FromIndex(a.tileIdx)
		a.paletteScroll, a.paletteScrollX = 0, 0
		a.clampScroll()
		return true
	}
	if inRect(mx, my, px+pad+tabW, yTabs, tabW, tabH) {
		a.pickTilesets = false
		a.pickCatalog = false
		a.clampSingleIdx()
		a.paletteScroll, a.paletteScrollX = 0, 0
		a.clampScroll()
		return true
	}
	if inRect(mx, my, px+pad+2*tabW, yTabs, tabRest, tabH) {
		a.pickTilesets = false
		a.pickCatalog = true
		a.clampCatItemIdx()
		a.editLayer = catalogItemLayer
		a.paletteScroll, a.paletteScrollX = 0, 0
		a.clampScroll()
		return true
	}

	// Смена набора (только тайлсеты)
	ySet := paletteYSetRow()
	if a.pickTilesets {
		btnW := 32
		if inRect(mx, my, px+pad, ySet, btnW, setRowH) {
			a.nextSet(-1)
			a.paletteScroll, a.paletteScrollX = 0, 0
			return true
		}
		if inRect(mx, my, px+pw-pad-btnW, ySet, btnW, setRowH) {
			a.nextSet(1)
			a.paletteScroll, a.paletteScrollX = 0, 0
			return true
		}
	}

	// Сетка / список
	if my < paletteGridTop() || my >= a.gridBottom() {
		return true // клик по палитре, но не по полю
	}

	if a.pickTilesets {
		n := a.tileCount()
		if n <= 0 {
			return true
		}
		cols := a.tilesetPaletteCols()
		step := a.thumbStep()
		relX := mx - (px + pad) + a.paletteScrollX
		relY := my - paletteGridTop() + a.paletteScroll
		if relY < 0 {
			return true
		}
		col := relX / step
		row := relY / step
		if col < 0 || col >= cols {
			return true
		}
		i := row*cols + col
		if i >= 0 && i < n {
			if !a.ctrlHeld() {
				a.setTileBrush1FromIndex(i)
			}
		}
		return true
	}

	if a.pickCatalog {
		n := len(a.catalogInteractIDs)
		if n <= 0 {
			return true
		}
		relY := my - paletteGridTop() + a.paletteScroll
		if relY < 0 {
			return true
		}
		i := relY / rowH
		if i >= 0 && i < n {
			a.catItemIdx = i
		}
		return true
	}

	keys := tiles.EditorSingleTextureKeys()
	if len(keys) == 0 {
		return true
	}
	relY := my - paletteGridTop() + a.paletteScroll
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
	pw := float32(a.paletteWidth)
	h := float32(a.winH)
	vector.DrawFilledRect(dst, px, 0, pw, h, color.RGBA{0x1e, 0x1e, 0x26, 0xff}, false)
	vector.StrokeLine(dst, px, 0, px, h, 1.5, color.RGBA{0x55, 0x55, 0x66, 0xff}, false)
	mx, my := ebiten.CursorPosition()
	if a.inPaletteSplitter(mx, my) {
		vector.DrawFilledRect(dst, px, 0, float32(paletteSplitterW), h, color.RGBA{0x44, 0x50, 0x68, 0x55}, false)
	}

	face := &textv2.GoTextFace{Source: ui.FontSource(), Size: 12}
	small := &textv2.GoTextFace{Source: ui.FontSource(), Size: 11}

	tex := a.texture()
	prevImg := tiles.ImageForTexture(a.catalogPreviewWireTexture())
	pvx := px + float32(a.paletteWidth/2-previewSize/2)
	pvy := float32(pad)
	pcx := pvx + float32(previewSize)/2
	pcy := pvy + float32(previewSize)/2
	vector.DrawFilledRect(dst, pvx-2, pvy-2, float32(previewSize)+4, float32(previewSize)+4, color.RGBA{0x30, 0x30, 0x3a, 0xff}, false)
	if prevImg != nil {
		tiles.DrawTextureScaledRotated(dst, prevImg, pcx, pcy, float32(previewSize), a.editRotation)
	} else {
		vector.DrawFilledRect(dst, pvx, pvy, float32(previewSize), float32(previewSize), color.RGBA{0x40, 0x40, 0x50, 0xff}, false)
	}

	btnWF := float32(32)
	yl := float32(paletteYLayerRow())
	vector.DrawFilledRect(dst, px+pad, yl, btnWF, float32(metaRowH), color.RGBA{0x3a, 0x3a, 0x48, 0xff}, false)
	vector.DrawFilledRect(dst, px+float32(a.paletteWidth-pad)-btnWF, yl, btnWF, float32(metaRowH), color.RGBA{0x3a, 0x3a, 0x48, 0xff}, false)
	opts := &textv2.DrawOptions{}
	opts.ColorScale.ScaleWithColor(color.RGBA{0xff, 0xff, 0xff, 0xff})
	opts.GeoM.Translate(float64(px+pad+9), float64(yl+4))
	textv2.Draw(dst, "◀", face, opts)
	opts.GeoM.Reset()
	opts.GeoM.Translate(float64(px+float32(a.paletteWidth-pad)-23), float64(yl+4))
	textv2.Draw(dst, "▶", face, opts)
	opts = &textv2.DrawOptions{}
	opts.PrimaryAlign = textv2.AlignCenter
	opts.GeoM.Translate(float64(px+float32(a.paletteWidth)/2), float64(yl+4))
	opts.ColorScale.ScaleWithColor(color.RGBA{0xd0, 0xd0, 0xd8, 0xff})
	textv2.Draw(dst, fmt.Sprintf("слой %d", a.editLayer), small, opts)

	yr := float32(paletteYRotRow())
	vector.DrawFilledRect(dst, px+pad, yr, btnWF, float32(metaRowH), color.RGBA{0x3a, 0x3a, 0x48, 0xff}, false)
	vector.DrawFilledRect(dst, px+float32(a.paletteWidth-pad)-btnWF, yr, btnWF, float32(metaRowH), color.RGBA{0x3a, 0x3a, 0x48, 0xff}, false)
	opts = &textv2.DrawOptions{}
	opts.ColorScale.ScaleWithColor(color.RGBA{0xff, 0xff, 0xff, 0xff})
	opts.GeoM.Translate(float64(px+pad+9), float64(yr+4))
	textv2.Draw(dst, "◀", face, opts)
	opts.GeoM.Reset()
	opts.GeoM.Translate(float64(px+float32(a.paletteWidth-pad)-23), float64(yr+4))
	textv2.Draw(dst, "▶", face, opts)
	q := tiles.NormalizeRotationQuarter(a.editRotation)
	opts = &textv2.DrawOptions{}
	opts.PrimaryAlign = textv2.AlignCenter
	opts.GeoM.Translate(float64(px+float32(a.paletteWidth)/2), float64(yr+4))
	opts.ColorScale.ScaleWithColor(color.RGBA{0xd0, 0xd0, 0xd8, 0xff})
	textv2.Draw(dst, fmt.Sprintf("↻ %d×90°", q), small, opts)

	yKey := float32(paletteYKey())
	opts = &textv2.DrawOptions{}
	opts.ColorScale.ScaleWithColor(color.RGBA{0xe8, 0xe8, 0xec, 0xff})
	opts.GeoM.Translate(float64(px+pad), float64(yKey))
	textv2.Draw(dst, tex, small, opts)

	yBlocks := float32(paletteYBlocks())
	btxt := "Коллизия: нет"
	if a.blocks {
		btxt = "Коллизия: да"
	}
	vector.DrawFilledRect(dst, px+pad, yBlocks, float32(a.paletteWidth-2*pad), float32(blocksH), color.RGBA{0x2a, 0x2a, 0x34, 0xff}, false)
	opts = &textv2.DrawOptions{}
	opts.ColorScale.ScaleWithColor(color.RGBA{0xcc, 0xcc, 0xd8, 0xff})
	opts.GeoM.Translate(float64(px+pad+6), float64(yBlocks+4))
	textv2.Draw(dst, btxt, face, opts)

	yTabs := float32(paletteYTabs())
	tabW := float32(a.paletteWidth-2*pad) / 3
	tabRest := float32(a.paletteWidth-2*pad) - 2*tabW
	tabHi := color.RGBA{0x38, 0x38, 0x48, 0xff}
	tabLo := color.RGBA{0x24, 0x24, 0x2e, 0xff}
	c0, c1, c2 := tabLo, tabLo, tabLo
	if a.pickTilesets {
		c0 = tabHi
	} else if a.pickCatalog {
		c2 = tabHi
	} else {
		c1 = tabHi
	}
	vector.DrawFilledRect(dst, px+pad, yTabs, tabW, float32(tabH), c0, false)
	vector.DrawFilledRect(dst, px+pad+tabW, yTabs, tabW, float32(tabH), c1, false)
	vector.DrawFilledRect(dst, px+pad+2*tabW, yTabs, tabRest, float32(tabH), c2, false)
	opts = &textv2.DrawOptions{}
	opts.ColorScale.ScaleWithColor(color.RGBA{0xee, 0xee, 0xf0, 0xff})
	opts.GeoM.Translate(float64(px+pad+6), float64(yTabs+4))
	textv2.Draw(dst, "Наборы", face, opts)
	opts.GeoM.Reset()
	opts.GeoM.Translate(float64(px+pad+tabW+4), float64(yTabs+4))
	textv2.Draw(dst, "Отдел.", face, opts)
	opts.GeoM.Reset()
	opts.GeoM.Translate(float64(px+pad+2*tabW+4), float64(yTabs+4))
	textv2.Draw(dst, "Предметы", face, opts)

	ySet := float32(paletteYSetRow())
	if a.pickTilesets {
		setName := a.currentSet()
		if len(setName) > 22 {
			setName = setName[:19] + "…"
		}
		btnW := float32(32)
		vector.DrawFilledRect(dst, px+pad, ySet, btnW, float32(setRowH), color.RGBA{0x3a, 0x3a, 0x48, 0xff}, false)
		vector.DrawFilledRect(dst, px+float32(a.paletteWidth-pad)-btnW, ySet, btnW, float32(setRowH), color.RGBA{0x3a, 0x3a, 0x48, 0xff}, false)
		opts = &textv2.DrawOptions{}
		opts.ColorScale.ScaleWithColor(color.RGBA{0xff, 0xff, 0xff, 0xff})
		opts.GeoM.Translate(float64(px+pad+9), float64(ySet+4))
		textv2.Draw(dst, "◀", face, opts)
		opts.GeoM.Reset()
		opts.GeoM.Translate(float64(px+float32(a.paletteWidth-pad)-23), float64(ySet+4))
		textv2.Draw(dst, "▶", face, opts)
		opts = &textv2.DrawOptions{}
		opts.PrimaryAlign = textv2.AlignCenter
		opts.GeoM.Translate(float64(px+float32(a.paletteWidth)/2), float64(ySet+4))
		opts.ColorScale.ScaleWithColor(color.RGBA{0xd0, 0xd0, 0xd8, 0xff})
		textv2.Draw(dst, setName, small, opts)
	} else if a.pickCatalog {
		opts = &textv2.DrawOptions{}
		opts.PrimaryAlign = textv2.AlignCenter
		opts.GeoM.Translate(float64(px+float32(a.paletteWidth)/2), float64(ySet+4))
		opts.ColorScale.ScaleWithColor(color.RGBA{0xc8, 0xd8, 0xf0, 0xff})
		line := "каталог · слой 1"
		if len(a.catalogInteractIDs) > 0 {
			id := a.catalogInteractIDs[a.catItemIdx]
			if len(id) > 28 {
				id = id[:25] + "…"
			}
			line = id + " · interact"
		}
		textv2.Draw(dst, line, small, opts)
	} else {
		opts = &textv2.DrawOptions{}
		opts.ColorScale.ScaleWithColor(color.RGBA{0x88, 0x88, 0x98, 0xff})
		opts.GeoM.Translate(float64(px+pad), float64(ySet+4))
		textv2.Draw(dst, "PNG в папке assets/", small, opts)
	}

	// Сетка миниатюр
	gy0 := float32(paletteGridTop())
	gh := float32(a.gridBottom() - paletteGridTop())
	vector.DrawFilledRect(dst, px+pad, gy0, float32(a.paletteWidth-2*pad), gh, color.RGBA{0x18, 0x18, 0x1e, 0xff}, false)

	if a.pickTilesets {
		n := a.tileCount()
		cols := a.tilesetPaletteCols()
		step := float32(a.thumbStep())
		base := a.currentSet()
		viewL := px + float32(pad)
		viewR := px + float32(a.paletteWidth-pad)
		for i := 0; i < n; i++ {
			key := tiles.TextureKey(base, i)
			img := tiles.ImageForTexture(key)
			col := i % cols
			row := i / cols
			cx := px + float32(pad) + float32(col)*step - float32(a.paletteScrollX)
			cy := gy0 + float32(row)*step - float32(a.paletteScroll)
			if cy+float32(thumbSize) < gy0 || cy > gy0+gh {
				continue
			}
			if cx+float32(thumbSize) < viewL || cx > viewR {
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
			if i == a.tileIdx && a.tileBrushW == 1 && a.tileBrushH == 1 {
				vector.StrokeRect(dst, cx-1, cy-1, float32(thumbSize)+2, float32(thumbSize)+2, 2, color.RGBA{0xff, 0xcc, 0x33, 0xff}, false)
			}
		}
		a.drawPaletteTileBrushRect(dst, px, gy0)
	} else if a.pickCatalog {
		for i, id := range a.catalogInteractIDs {
			cy := gy0 + float32(i*rowH) - float32(a.paletteScroll)
			if cy+float32(rowH-4) < gy0 || cy > gy0+gh {
				continue
			}
			cx := px + float32(pad)
			tsz := float32(thumbSize)
			vector.DrawFilledRect(dst, cx, cy+4, tsz, tsz, color.RGBA{0x2a, 0x32, 0x40, 0xff}, false)
			wire := id
			if gamecontent.IsInvisibleTriggerCatalogID(id) {
				wire = gamekit.InvisibleTileTextureKey
			}
			img := tiles.ImageForTexture(wire)
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
			opts.ColorScale.ScaleWithColor(color.RGBA{0xcc, 0xd8, 0xf0, 0xff})
			opts.GeoM.Translate(float64(cx+tsz+8), float64(cy+10))
			textv2.Draw(dst, id, face, opts)
			if i == a.catItemIdx {
				rw := float32(a.paletteWidth - 2*pad + 2)
				vector.StrokeRect(dst, cx-1, cy+3, rw, float32(thumbSize)+2, 2, color.RGBA{0xff, 0xcc, 0x33, 0xff}, false)
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
				rw := float32(a.paletteWidth - 2*pad + 2)
				vector.StrokeRect(dst, cx-1, cy+3, rw, float32(thumbSize)+2, 2, color.RGBA{0xff, 0xcc, 0x33, 0xff}, false)
			}
		}
	}
}

func (a *App) drawPaletteTileBrushRect(dst *ebiten.Image, px, gy0 float32) {
	n := a.tileCount()
	if n <= 0 {
		return
	}
	step := float32(a.thumbStep())
	c0, r0 := a.tileBrushCol0, a.tileBrushRow0
	wb, hb := a.tileBrushW, a.tileBrushH
	if a.palBrushDrag {
		c0 = min(a.palDragSC, a.palDragEC)
		c1 := max(a.palDragSC, a.palDragEC)
		r0 = min(a.palDragSR, a.palDragER)
		r1 := max(a.palDragSR, a.palDragER)
		wb = c1 - c0 + 1
		hb = r1 - r0 + 1
	}
	rx := px + float32(pad) + float32(c0)*step - float32(a.paletteScrollX) - 2
	ry := gy0 + float32(r0)*step - float32(a.paletteScroll) - 2
	rw := float32(wb)*step + 4
	rh := float32(hb)*step + 4
	vector.StrokeRect(dst, rx, ry, rw, rh, 2.5, color.RGBA{0x58, 0xc8, 0xf0, 0xee}, false)
}

func (a *App) tickPaletteBrushDrag(mx, my int) {
	if !a.pickTilesets || a.paletteDrag {
		return
	}
	col, row, ok := a.paletteTilesetGridCell(mx, my)
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && a.ctrlHeld() && ok {
		a.palBrushDrag = true
		a.palDragSC, a.palDragSR = col, row
		a.palDragEC, a.palDragER = col, row
	}
	if a.palBrushDrag && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && a.ctrlHeld() {
		if ok {
			a.palDragEC, a.palDragER = col, row
		}
	}
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) && a.palBrushDrag {
		a.palBrushDrag = false
		a.finalizePaletteBrushRect()
	}
}

func (a *App) paletteTilesetGridCell(mx, my int) (col, row int, ok bool) {
	if !a.inPalette(mx, my) || !a.pickTilesets {
		return 0, 0, false
	}
	if my < paletteGridTop() || my >= a.gridBottom() {
		return 0, 0, false
	}
	px := a.paletteX()
	step := a.thumbStep()
	cols := a.tilesetPaletteCols()
	if cols <= 0 || step <= 0 {
		return 0, 0, false
	}
	relX := mx - (px + pad) + a.paletteScrollX
	relY := my - paletteGridTop() + a.paletteScroll
	if relY < 0 {
		return 0, 0, false
	}
	col = relX / step
	row = relY / step
	if col < 0 || col >= cols {
		return 0, 0, false
	}
	i := row*cols + col
	if i < 0 || i >= a.tileCount() {
		return 0, 0, false
	}
	return col, row, true
}

func (a *App) finalizePaletteBrushRect() {
	c0 := min(a.palDragSC, a.palDragEC)
	c1 := max(a.palDragSC, a.palDragEC)
	r0 := min(a.palDragSR, a.palDragER)
	r1 := max(a.palDragSR, a.palDragER)
	a.tileBrushCol0 = c0
	a.tileBrushRow0 = r0
	a.tileBrushW = c1 - c0 + 1
	a.tileBrushH = r1 - r0 + 1
	a.clampTileBrushToGrid()
}

func (a *App) mapTileFromCursor(mx, my int) (tx, ty int, ok bool) {
	if mx >= a.paletteX() {
		return 0, 0, false
	}
	return world.TileFromScreenWithCamZoom(mx, my, a.camX, a.camY, a.camZoomEffective())
}
