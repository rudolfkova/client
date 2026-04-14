package tiles

import (
	"io/fs"
	"log"
	"path/filepath"
	"slices"
	"strings"

	"client/data"
)

// editorTileSetBases — PNG в корне assets/tileSets/ (без .png), по алфавиту.
var editorTileSetBases []string

// editorAnimSetBases — PNG в assets/tileSets/anim/…, ключ base = путь без .png (например anim/gentle … v01).
var editorAnimSetBases []string

// singleTextureFiles wire-ключ → путь во встроенном FS (например assets/Grass_Middle.png).
var singleTextureFiles map[string]string

func init() {
	sets, animBases, singles, err := discoverTileAssets()
	if err != nil {
		log.Printf("tiles: сканирование assets: %v", err)
	}
	if sets == nil {
		sets = []string{}
	}
	if animBases == nil {
		animBases = []string{}
	}
	if singles == nil {
		singles = make(map[string]string)
	}
	editorTileSetBases = sets
	editorAnimSetBases = animBases
	singleTextureFiles = singles

	for _, base := range editorTileSetBases {
		path := "assets/tileSets/" + base + ".png"
		if _, err := registerTilesetPNG(path, base); err != nil {
			log.Printf("tiles: тайлсет %s: %v", base, err)
			setMu.Lock()
			setLoadFailed[base] = struct{}{}
			setMu.Unlock()
		}
	}
	for _, base := range editorAnimSetBases {
		path := "assets/tileSets/" + base + ".png"
		if err := RegisterAnimTilesetFromEmbed(path, base); err != nil {
			log.Printf("tiles: anim %s: %v", base, err)
			setMu.Lock()
			setLoadFailed[base] = struct{}{}
			setMu.Unlock()
		}
	}
}

func discoverTileAssets() (tileSets []string, animBases []string, singles map[string]string, err error) {
	singles = make(map[string]string)
	setSeen := make(map[string]struct{})
	animSeen := make(map[string]struct{})

	err = fs.WalkDir(data.TileAssets, ".", func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		path = filepath.ToSlash(path)
		if !strings.HasSuffix(strings.ToLower(path), ".png") {
			return nil
		}

		switch {
		case strings.HasPrefix(path, "assets/tileSets/"):
			rel := path[len("assets/tileSets/"):]
			if strings.HasPrefix(rel, "anim/") {
				base := strings.TrimSuffix(rel, ".png")
				if base != "" && base != "anim" {
					animSeen[base] = struct{}{}
				}
				return nil
			}
			if strings.Contains(rel, "/") {
				return nil
			}
			base := strings.TrimSuffix(rel, ".png")
			if base == "" {
				return nil
			}
			setSeen[base] = struct{}{}

		case strings.HasPrefix(path, "assets/"):
			rel := path[len("assets/"):]
			if strings.Contains(rel, "/") {
				return nil
			}
			key := strings.TrimSuffix(rel, ".png")
			if key == "" {
				return nil
			}
			singles[key] = path
		}
		return nil
	})
	if err != nil {
		return nil, nil, singles, err
	}

	tileSets = make([]string, 0, len(setSeen))
	for b := range setSeen {
		tileSets = append(tileSets, b)
	}
	slices.Sort(tileSets)

	animBases = make([]string, 0, len(animSeen))
	for b := range animSeen {
		animBases = append(animBases, b)
	}
	slices.Sort(animBases)

	return tileSets, animBases, singles, nil
}
