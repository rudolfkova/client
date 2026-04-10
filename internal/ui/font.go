package ui

import (
	"bytes"
	"log"
	"sync"

	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	textv2 "github.com/hajimehoshi/ebiten/v2/text/v2"
)

var (
	once   sync.Once
	source *textv2.GoTextFaceSource
)

func FontSource() *textv2.GoTextFaceSource {
	once.Do(func() {
		s, err := textv2.NewGoTextFaceSource(bytes.NewReader(fonts.MPlus1pRegular_ttf))
		if err != nil {
			log.Fatal(err)
		}
		source = s
	})
	return source
}
