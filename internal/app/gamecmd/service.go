package gamecmd

import "github.com/rudolfkova/grpc_auth/pkg/gamekit"

// Writer - порт отправки интентов в game websocket.
type Writer interface {
	Send(typ string, payload any) error
}

type Service struct {
	writer Writer
}

func NewService(writer Writer) *Service {
	return &Service{writer: writer}
}

func (s *Service) Move(dx, dy int) error {
	return s.writer.Send(gamekit.TypeMove, gamekit.MoveIntent{DX: dx, DY: dy})
}

func (s *Service) Hit(targetID int64, damage int) error {
	return s.writer.Send(gamekit.TypeHit, gamekit.HitIntent{TargetID: targetID, Damage: damage})
}

func (s *Service) Interact(intent gamekit.InteractIntent) error {
	return s.writer.Send(gamekit.TypeInteract, intent)
}

func (s *Service) Pickup(intent gamekit.PickupIntent) error {
	return s.writer.Send(gamekit.TypePickupItem, intent)
}

func (s *Service) DropItem(from string) error {
	return s.writer.Send(gamekit.TypeDropItem, gamekit.DropItemIntent{From: from})
}

func (s *Service) InventoryMove(from, to string) error {
	return s.writer.Send(gamekit.TypeInventoryMove, gamekit.InventoryMoveIntent{From: from, To: to})
}

func (s *Service) SpawnTile(intent gamekit.TileSpawnIntent) error {
	return s.writer.Send(gamekit.TypeSpawnTile, intent)
}

func (s *Service) ClearTile(intent gamekit.TileClearIntent) error {
	return s.writer.Send(gamekit.TypeClearTile, intent)
}

func (s *Service) SaveWorld(name string) error {
	return s.writer.Send(gamekit.TypeSaveWorld, gamekit.SaveWorldIntent{Name: name})
}
