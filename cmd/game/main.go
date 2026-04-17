package main

import (
	"log"

	"client/internal/bootstrap/game"
)

func main() {
	if err := game.Run(); err != nil {
		log.Fatal(err)
	}
}
