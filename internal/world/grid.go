package world

const (
	TileSize      = 48
	GridPad       = 40.0
	PlayerRadius  = 18.0
	LabelTextSize = 16.0
	LabelAboveGap = 6.0
)

func ToScreen(tileX, tileY int) (cx, cy float32) {
	cx = float32(GridPad) + float32(tileX)*TileSize + TileSize*0.5
	cy = float32(GridPad) + float32(tileY)*TileSize + TileSize*0.5
	return cx, cy
}

// TileFromScreen возвращает координаты клетки под курсором; ok=false если вне сетки по осям.
func TileFromScreen(mx, my int) (tx, ty int, ok bool) {
	return TileFromScreenWithCam(mx, my, 0, 0)
}

// TileFromScreenWithCam — то же с учётом смещения камеры (мир в px: +cam = сдвиг взгляда вправо/вниз).
func TileFromScreenWithCam(mx, my int, camX, camY float32) (tx, ty int, ok bool) {
	fx := float32(mx) + camX
	fy := float32(my) + camY
	if fx < float32(GridPad) || fy < float32(GridPad) {
		return 0, 0, false
	}
	tx = int((fx - float32(GridPad)) / TileSize)
	ty = int((fy - float32(GridPad)) / TileSize)
	if tx < 0 || ty < 0 {
		return 0, 0, false
	}
	return tx, ty, true
}
