package data

import "embed"

//go:embed all:assets
var TileAssets embed.FS

//go:embed all:anim
var AnimAssets embed.FS

//go:embed content/catalog.json
var ContentCatalogJSON []byte
