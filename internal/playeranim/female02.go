package playeranim

import "github.com/hajimehoshi/ebiten/v2"

const female012SpriteID = "Female 01-2"

// Female012Sheet — лист в data/anim/Female 01-2/Female 01-2.png.
func Female012Sheet() *ebiten.Image {
	return WalkSheet(female012SpriteID)
}

// DrawFemale012 — тот же расклад кадров, что у Male 01-1 (32×32, 4 ряда по сторонам).
func DrawFemale012(dst *ebiten.Image, cx, cy float32, cardinal int, walking bool, phase float64, scale float64) {
	DrawWalkSheet(dst, female012SpriteID, cx, cy, cardinal, walking, phase, scale)
}
