package characterweb

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"client/internal/characterv1"

	"github.com/rudolfkova/grpc_auth/pkg/gamekit"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Server раздаёт статику редактора и JSON-API к character-service по gRPC.
type Server struct {
	token    string
	grpcAddr string
	client   characterv1.CharacterServiceClient
	conn     *grpc.ClientConn
	// dataRoot — абсолютный путь к каталогу data (как у -data); пусто — без сканирования anim.
	dataRoot string
}

func New(grpcAddr, serviceToken, dataRoot string) (*Server, error) {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Server{
		token:    strings.TrimSpace(serviceToken),
		grpcAddr: grpcAddr,
		client:   characterv1.NewCharacterServiceClient(conn),
		conn:     conn,
		dataRoot: dataRoot,
	}, nil
}

func (s *Server) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (s *Server) ctx(ctx context.Context) context.Context {
	if s.token == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "x-service-token", s.token)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("characterweb: encode json: %v", err)
	}
}

func grpcErrMsg(err error) string {
	st, ok := status.FromError(err)
	if !ok {
		return err.Error()
	}
	return st.Message()
}

func grpcHTTPStatus(err error) int {
	st, ok := status.FromError(err)
	if !ok {
		return http.StatusBadGateway
	}
	switch st.Code() {
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.NotFound:
		return http.StatusNotFound
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	default:
		return http.StatusBadGateway
	}
}

func characterView(c *characterv1.Character) (map[string]any, error) {
	if c == nil {
		return nil, nil
	}
	play := gamekit.ParseCharacterPlayData(c.GetData())
	playJSON, err := json.Marshal(play)
	if err != nil {
		return nil, err
	}
	var playObj any
	if err := json.Unmarshal(playJSON, &playObj); err != nil {
		return nil, err
	}
	return map[string]any{
		"id":              c.GetId(),
		"user_id":         c.GetUserId(),
		"display_name":    c.GetDisplayName(),
		"description":     c.GetDescription(),
		"schema_version":  c.GetSchemaVersion(),
		"version":         c.GetVersion(),
		"created_at_unix": c.GetCreatedAtUnix(),
		"updated_at_unix": c.GetUpdatedAtUnix(),
		"play_data":       playObj,
	}, nil
}

// Register mounts API and static file handler for the editor UI.
func (s *Server) Register(mux *http.ServeMux, static http.Handler) {
	mux.HandleFunc("POST /api/list", s.handleList)
	mux.HandleFunc("POST /api/get", s.handleGet)
	mux.HandleFunc("POST /api/create", s.handleCreate)
	mux.HandleFunc("POST /api/save-data", s.handleSaveData)
	mux.HandleFunc("POST /api/delete", s.handleDelete)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "grpc": s.grpcAddr})
	})
	mux.HandleFunc("GET /api/anims", s.handleListAnims)
	mux.Handle("/", static)
}

func (s *Server) handleListAnims(w http.ResponseWriter, r *http.Request) {
	if s.dataRoot == "" {
		writeJSON(w, map[string]any{
			"sprites": []string{},
			"hint":    "укажите -data путь к каталогу data репозитория (там лежит anim/)",
		})
		return
	}
	list, err := ListAnimSpriteIDs(s.dataRoot)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"sprites": list})
}

type listReq struct {
	UserID int64 `json:"user_id"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	var req listReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "json: "+err.Error())
		return
	}
	if req.UserID == 0 {
		writeErr(w, http.StatusBadRequest, "user_id required")
		return
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}
	resp, err := s.client.ListCharacters(s.ctx(r.Context()), &characterv1.ListCharactersRequest{
		UserId: req.UserID,
		Limit:  req.Limit,
		Offset: req.Offset,
	})
	if err != nil {
		writeErr(w, grpcHTTPStatus(err), grpcErrMsg(err))
		return
	}
	out := make([]map[string]any, 0, len(resp.GetCharacters()))
	for _, c := range resp.GetCharacters() {
		out = append(out, map[string]any{
			"id":             c.GetId(),
			"display_name":   c.GetDisplayName(),
			"version":        c.GetVersion(),
			"schema_version": c.GetSchemaVersion(),
		})
	}
	writeJSON(w, map[string]any{"characters": out})
}

type getReq struct {
	UserID      int64  `json:"user_id"`
	CharacterID string `json:"character_id"`
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	var req getReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "json: "+err.Error())
		return
	}
	if req.UserID == 0 || req.CharacterID == "" {
		writeErr(w, http.StatusBadRequest, "user_id and character_id required")
		return
	}
	resp, err := s.client.GetCharacter(s.ctx(r.Context()), &characterv1.GetCharacterRequest{
		UserId: req.UserID,
		Id:     req.CharacterID,
	})
	if err != nil {
		writeErr(w, grpcHTTPStatus(err), grpcErrMsg(err))
		return
	}
	v, err := characterView(resp.GetCharacter())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"character": v})
}

type createReq struct {
	UserID        int64           `json:"user_id"`
	DisplayName   string          `json:"display_name"`
	Description   string          `json:"description"`
	CharacterID   string          `json:"character_id"` // optional UUID; empty → server generates
	PlayData      json.RawMessage `json:"play_data"`
	SchemaVersion int32           `json:"schema_version"`
}

func playDataBytes(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		d := gamekit.NewDefaultCharacterPlayData()
		return gamekit.MarshalCharacterPlayData(d)
	}
	d := gamekit.ParseCharacterPlayData(raw)
	return gamekit.MarshalCharacterPlayData(d)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "json: "+err.Error())
		return
	}
	if req.UserID == 0 || strings.TrimSpace(req.DisplayName) == "" {
		writeErr(w, http.StatusBadRequest, "user_id and display_name required")
		return
	}
	data, err := playDataBytes(req.PlayData)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "play_data: "+err.Error())
		return
	}
	sv := req.SchemaVersion
	if sv == 0 {
		sv = gamekit.CharacterDataSchemaVersion
	}
	resp, err := s.client.CreateCharacter(s.ctx(r.Context()), &characterv1.CreateCharacterRequest{
		UserId:        req.UserID,
		Id:            strings.TrimSpace(req.CharacterID),
		DisplayName:   strings.TrimSpace(req.DisplayName),
		Description:   req.Description,
		Data:          data,
		SchemaVersion: sv,
	})
	if err != nil {
		writeErr(w, grpcHTTPStatus(err), grpcErrMsg(err))
		return
	}
	v, err := characterView(resp.GetCharacter())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"character": v})
}

type saveDataReq struct {
	UserID          int64           `json:"user_id"`
	CharacterID     string          `json:"character_id"`
	PlayData        json.RawMessage `json:"play_data"`
	SchemaVersion   int32           `json:"schema_version"`
	ExpectedVersion int64           `json:"expected_version"`
}

func (s *Server) handleSaveData(w http.ResponseWriter, r *http.Request) {
	var req saveDataReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "json: "+err.Error())
		return
	}
	if req.UserID == 0 || req.CharacterID == "" {
		writeErr(w, http.StatusBadRequest, "user_id and character_id required")
		return
	}
	data, err := playDataBytes(req.PlayData)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "play_data: "+err.Error())
		return
	}
	sv := req.SchemaVersion
	if sv == 0 {
		sv = gamekit.CharacterDataSchemaVersion
	}
	resp, err := s.client.ReplaceCharacterData(s.ctx(r.Context()), &characterv1.ReplaceCharacterDataRequest{
		UserId:          req.UserID,
		Id:              req.CharacterID,
		Data:            data,
		SchemaVersion:   sv,
		ExpectedVersion: req.ExpectedVersion,
	})
	if err != nil {
		writeErr(w, grpcHTTPStatus(err), grpcErrMsg(err))
		return
	}
	v, err := characterView(resp.GetCharacter())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"character": v})
}

type deleteReq struct {
	UserID      int64  `json:"user_id"`
	CharacterID string `json:"character_id"`
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req deleteReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "json: "+err.Error())
		return
	}
	if req.UserID == 0 || req.CharacterID == "" {
		writeErr(w, http.StatusBadRequest, "user_id and character_id required")
		return
	}
	_, err := s.client.DeleteCharacter(s.ctx(r.Context()), &characterv1.DeleteCharacterRequest{
		UserId: req.UserID,
		Id:     req.CharacterID,
	})
	if err != nil {
		writeErr(w, grpcHTTPStatus(err), grpcErrMsg(err))
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
