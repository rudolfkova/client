package playeranim

import "github.com/hajimehoshi/ebiten/v2"

const male01SpriteID = "Male 01-1"

// Male01Sheet загружает лист один раз; при ошибке nil.
func Male01Sheet() *ebiten.Image {
	return WalkSheet(male01SpriteID)
}

// WalkSpriteRow: строка листа — ряд 0 вниз, 1 влево, 2 вправо, 3 вверх.
func WalkSpriteRow(cardinal int) int {
	switch cardinal {
	case CardinalS:
		return 0
	case CardinalW:
		return 1
	case CardinalE:
		return 2
	case CardinalN:
		return 3
	default:
		return 2
	}
}

// WalkFrameCol: стоит — средний кадр (1); идёт — цикл 0→1→2→1.
func WalkFrameCol(walking bool, phase float64) int {
	if !walking {
		return 1
	}
	i := int(phase) % 4
	return [...]int{0, 1, 2, 1}[i]
}

// DrawMale01 рисует кадр по центру (cx, cy); scale — множитель к размеру кадра.
func DrawMale01(dst *ebiten.Image, cx, cy float32, cardinal int, walking bool, phase float64, scale float64) {
	DrawWalkSheet(dst, male01SpriteID, cx, cy, cardinal, walking, phase, scale)
}
