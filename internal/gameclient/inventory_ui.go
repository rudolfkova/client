package gameclient

import (
	"fmt"
	"image/color"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	textv2 "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/rudolfkova/grpc_auth/pkg/gamekit"

	"client/data"
	"client/internal/gamecontent"
	"client/internal/gamews"
	"client/internal/tiles"
	"client/internal/ui"
)

// Порядок слотов слева направо в панели.
var invSlotBarOrder = []string{
	gamekit.InvSlotArmor,
	gamekit.InvSlotAccessory1,
	gamekit.InvSlotAccessory2,
	gamekit.InvSlotHandMain,
	gamekit.InvSlotHandOff,
	gamekit.InvSlotBackpackPref + "0",
	gamekit.InvSlotBackpackPref + "1",
	gamekit.InvSlotBackpackPref + "2",
	gamekit.InvSlotBackpackPref + "3",
	gamekit.InvSlotBackpackPref + "4",
}

func invSlotShortTitle(key string) string {
	switch key {
	case gamekit.InvSlotArmor:
		return "Броня"
	case gamekit.InvSlotAccessory1:
		return "Акс1"
	case gamekit.InvSlotAccessory2:
		return "Акс2"
	case gamekit.InvSlotHandMain:
		return "Рука"
	case gamekit.InvSlotHandOff:
		return "Левая"
	default:
		if strings.HasPrefix(key, gamekit.InvSlotBackpackPref) {
			return "Р" + strings.TrimPrefix(key, gamekit.InvSlotBackpackPref)
		}
		return key
	}
}

func invItemLabel(jsonRaw []byte, itemDefID string) string {
	itemDefID = strings.TrimSpace(itemDefID)
	if itemDefID == "" {
		return "—"
	}
	n := gamecontent.ItemDisplayName(jsonRaw, itemDefID)
	if n == "" {
		n = itemDefID
	}
	if utf8.RuneCountInString(n) > 10 {
		n = string([]rune(n)[:9]) + "…"
	}
	return n
}

type invSlotRect struct {
	key    string
	x0, y0 int
	w, h   int
}

func (g *Game) inventorySlotRects(ww, wh int) []invSlotRect {
	const (
		slotW  = 72
		slotH  = 44
		gap    = 4
		titleH = 18
	)
	n := len(invSlotBarOrder)
	barW := n*slotW + (n-1)*gap
	xStart := (ww - barW) / 2
	if xStart < 4 {
		xStart = 4
	}
	y0 := wh - titleH - slotH - 8
	if y0 < 0 {
		y0 = 0
	}
	out := make([]invSlotRect, 0, n)
	for i, key := range invSlotBarOrder {
		x := xStart + i*(slotW+gap)
		out = append(out, invSlotRect{key: key, x0: x, y0: y0, w: slotW, h: slotH})
	}
	return out
}

// consumeInventoryBarRMB — ПКМ по полосе слотов: выбросить предмет (drop_item), не пропуская клик в мир.
// Возвращает true, если курсор был над панелью инвентаря (клик «съеден»).
func (g *Game) consumeInventoryBarRMB(ww, wh int) bool {
	if g.lobbyFocused {
		return false
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		return false
	}
	mx, my := ebiten.CursorPosition()
	for _, r := range g.inventorySlotRects(ww, wh) {
		if mx >= r.x0 && mx < r.x0+r.w && my >= r.y0 && my < r.y0+r.h {
			g.tryDropFromSlot(r.key)
			return true
		}
	}
	return false
}

func (g *Game) tryDropFromSlot(slot string) {
	pl, ok := g.World.Players[g.userID]
	if !ok {
		return
	}
	if strings.TrimSpace(slotItemID(pl.Inventory, slot)) == "" {
		return
	}
	if err := gamews.Send(g.wsGame, gamekit.TypeDropItem, gamekit.DropItemIntent{From: slot}); err != nil {
		log.Printf("ws drop_item: %v", err)
	}
	g.invPickSlot = ""
}

func (g *Game) handleInventoryBarInput(ww, wh int) {
	if g.lobbyFocused {
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.invPickSlot = ""
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	mx, my := ebiten.CursorPosition()
	for _, r := range g.inventorySlotRects(ww, wh) {
		if mx >= r.x0 && mx < r.x0+r.w && my >= r.y0 && my < r.y0+r.h {
			g.onInventorySlotClick(r.key)
			return
		}
	}
}

func (g *Game) onInventorySlotClick(slot string) {
	pl, ok := g.World.Players[g.userID]
	if !ok {
		g.invPickSlot = ""
		return
	}
	if g.invPickSlot == "" {
		g.invPickSlot = slot
		return
	}
	if g.invPickSlot == slot {
		g.invPickSlot = ""
		return
	}
	from, to := g.invPickSlot, slot
	invCopy := pl.Inventory
	if !gamekit.TrySwapInventorySlots(&invCopy, from, to, func(id string) bool {
		return gamecontent.ItemCanPlaceInBackpack(data.ContentCatalogJSON, id)
	}) {
		g.invPickSlot = ""
		return
	}
	if err := gamews.Send(g.wsGame, gamekit.TypeInventoryMove, gamekit.InventoryMoveIntent{From: from, To: to}); err != nil {
		log.Printf("ws inventory_move: %v", err)
	}
	g.invPickSlot = ""
}

func (g *Game) drawInventoryBar(screen *ebiten.Image, ww, wh int) {
	pl, ok := g.World.Players[g.userID]
	if !ok {
		return
	}
	const titleH = 18
	yTitle := float32(wh - 44 - titleH - 8)
	if yTitle < 0 {
		yTitle = 0
	}
	titleFace := &textv2.GoTextFace{Source: ui.FontSource(), Size: 12}
	hint := "Инвентарь: ЛКМ×2 — обмен слотов; ПКМ по слоту с предметом — выбросить"
	if g.invPickSlot != "" {
		hint = fmt.Sprintf("Выбран «%s» — другой слот (ЛКМ) или Esc; ПКМ по любому слоту с предметом — выбросить", invSlotShortTitle(g.invPickSlot))
	}
	tw, _ := textv2.Measure(hint, titleFace, 0)
	tx := float32(ww/2) - float32(tw)/2
	if tx < 4 {
		tx = 4
	}
	to := &textv2.DrawOptions{}
	to.GeoM.Translate(float64(tx), float64(yTitle))
	to.ColorScale.ScaleWithColor(color.RGBA{0xc8, 0xd0, 0xe8, 0xff})
	textv2.Draw(screen, hint, titleFace, to)

	lineFace := &textv2.GoTextFace{Source: ui.FontSource(), Size: 10}
	for _, r := range g.inventorySlotRects(ww, wh) {
		sel := g.invPickSlot == r.key
		fill := color.RGBA{0x1c, 0x20, 0x2e, 0xea}
		if sel {
			fill = color.RGBA{0x2a, 0x34, 0x4a, 0xf0}
		}
		vector.DrawFilledRect(screen, float32(r.x0), float32(r.y0), float32(r.w), float32(r.h), fill, false)
		st := color.RGBA{0x5a, 0x68, 0x88, 0xff}
		if sel {
			st = color.RGBA{0xe8, 0xc8, 0x60, 0xff}
		}
		vector.StrokeRect(screen, float32(r.x0), float32(r.y0), float32(r.w), float32(r.h), 1.5, st, false)

		itemID := slotItemID(pl.Inventory, r.key)
		lab := invSlotShortTitle(r.key)
		lo := &textv2.DrawOptions{}
		lo.GeoM.Translate(float64(r.x0)+4, float64(r.y0)+2)
		lo.ColorScale.ScaleWithColor(color.RGBA{0xa8, 0xb0, 0xc8, 0xff})
		textv2.Draw(screen, lab, lineFace, lo)
		lo.GeoM.Reset()
		lo.GeoM.Translate(float64(r.x0)+4, float64(r.y0)+16)
		lo.ColorScale.ScaleWithColor(color.RGBA{0xe8, 0xec, 0xf8, 0xff})
		textv2.Draw(screen, invItemLabel(data.ContentCatalogJSON, itemID), lineFace, lo)

		wire := tiles.ResolvedItemTexture(itemID, func(id string) bool {
			return gamecontent.IsCatalogItemID(data.ContentCatalogJSON, id)
		})
		if img := tiles.ImageForTexture(wire); img != nil {
			cx := float32(r.x0+r.w) - 22
			cy := float32(r.y0+r.h) - 22
			tiles.DrawTextureScaledRotated(screen, img, cx, cy, 20, 0)
		}
	}
}

func slotItemID(inv gamekit.PlayerInventory, slot string) string {
	switch slot {
	case gamekit.InvSlotArmor:
		return inv.Armor
	case gamekit.InvSlotAccessory1:
		return inv.Accessory1
	case gamekit.InvSlotAccessory2:
		return inv.Accessory2
	case gamekit.InvSlotHandMain:
		return inv.HandMain
	case gamekit.InvSlotHandOff:
		return inv.HandOff
	default:
		if strings.HasPrefix(slot, gamekit.InvSlotBackpackPref) {
			suf := slot[len(gamekit.InvSlotBackpackPref):]
			if len(suf) == 1 {
				i := int(suf[0] - '0')
				if i >= 0 && i < len(inv.Backpack) {
					return inv.Backpack[i]
				}
			}
		}
	}
	return ""
}
