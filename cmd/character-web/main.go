// Character-web: локальный HTTP-сервер со статикой редактора и JSON-API к character-service (gRPC).
//
// Запуск из корня репозитория client:
//
//	go run ./cmd/character-web -grpc 127.0.0.1:50055 -token "$CHARACTER_WEB_SERVICE_TOKEN"
//
// Открыть http://127.0.0.1:8765/ — ввести user_id, токен задаётся флагом (не храните в публичном репо).
package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"client/internal/characterweb"
)

//go:embed all:web
var webFS embed.FS

func main() {
	listen := flag.String("listen", "127.0.0.1:8765", "HTTP listen address")
	grpcAddr := flag.String("grpc", "127.0.0.1:50055", "character-service gRPC host:port")
	token := flag.String("token", os.Getenv("CHARACTER_WEB_SERVICE_TOKEN"), "x-service-token (or env CHARACTER_WEB_SERVICE_TOKEN)")
	dataDir := flag.String("data", "data", "optional: static files from this directory at /assets/")

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()

	if *token == "" {
		log.Println("warning: empty -token; set if character-service requires service_token")
	}

	absData := ""
	if p, err := filepath.Abs(*dataDir); err == nil {
		if fi, e := os.Stat(p); e == nil && fi.IsDir() {
			absData = p
		}
	}

	srv, err := characterweb.New(*grpcAddr, *token, absData)
	if err != nil {
		log.Fatal(err)
	}
	defer srv.Close()

	sub, err := fs.Sub(webFS, "web/character-editor")
	if err != nil {
		log.Fatal(err)
	}
	static := http.FileServer(http.FS(sub))

	mux := http.NewServeMux()
	srv.Register(mux, static)

	if absData != "" {
		mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(absData))))
		log.Printf("serving %s at /assets/ (GET /api/anims — список из anim/)", absData)
	}

	log.Printf("character editor http://%s/  →  gRPC %s", *listen, *grpcAddr)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatal(err)
	}
}
