package state

import "client/internal/core/worldstate"

// World оставлен как совместимый алиас; новая реализация в internal/core/worldstate.
type World = worldstate.World

func NewWorld() *World {
	return worldstate.NewWorld()
}
