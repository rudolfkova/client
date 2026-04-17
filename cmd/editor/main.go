package main

import (
	"log"

	"client/internal/bootstrap/editor"
)

func main() {
	if err := editor.Run(); err != nil {
		log.Fatal(err)
	}
}
