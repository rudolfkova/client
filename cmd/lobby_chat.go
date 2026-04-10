package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// lobbyChatDesiredID — ожидаемый id группового чата лобби.
// Если в этот чат вступить не удаётся (нет строки в БД и т.п.), клиент создаёт новый чат (POST /chat/create) и подключает пользователя.
// Поставьте 0, чтобы каждый запуск создавал отдельный чат (удобно для отладки).
const lobbyChatDesiredID int64 = 1

const lobbyChatCreateName = "Lobby"

const (
	maxLobbyChatLines  = 120
	maxLobbyDraftRunes = 512
)

// LobbyChatMessage — push с /ws/subscribe (не envelope формата game).
type LobbyChatMessage struct {
	ID        int64     `json:"id"`
	ChatID    int64     `json:"chat_id"`
	SenderID  int64     `json:"sender_id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type chatLine struct {
	id       int64
	senderID int64
	text     string
}

func userIDFromAccessJWT(access string) (int64, error) {
	parts := strings.Split(access, ".")
	if len(parts) != 3 {
		return 0, fmt.Errorf("jwt: expected 3 segments")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, fmt.Errorf("jwt payload: %w", err)
	}
	var claims struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return 0, err
	}
	if claims.UserID == 0 {
		return 0, fmt.Errorf("jwt: missing user_id")
	}
	return claims.UserID, nil
}

// joinLobbyChat пытается добавить пользователя в чат. Возвращает HTTP-статус и ошибку с телом ответа при статусе != 200.
func joinLobbyChat(token string, userID, chatID int64) (status int, err error) {
	body, err := json.Marshal(map[string]int64{
		"chat_id": chatID,
		"user_id": userID,
	})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, authBaseURL+"/chat/members", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return resp.StatusCode, nil
}

func createLobbyChat(token, name string) (chatID int64, err error) {
	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, authBaseURL+"/chat/create", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("create chat %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var out struct {
		ChatID int64 `json:"chat_id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, err
	}
	if out.ChatID == 0 {
		return 0, fmt.Errorf("create chat: missing chat_id in response")
	}
	return out.ChatID, nil
}

// ensureLobbyChat вступает в чат lobbyChatDesiredID или создаёт новый групповой чат и вступает в него.
func ensureLobbyChat(token string, userID int64) (chatID int64, err error) {
	if lobbyChatDesiredID > 0 {
		st, jerr := joinLobbyChat(token, userID, lobbyChatDesiredID)
		if jerr == nil && st == http.StatusOK {
			return lobbyChatDesiredID, nil
		}
		if st == http.StatusUnauthorized {
			return 0, fmt.Errorf("lobby join: %w", jerr)
		}
		log.Printf("lobby: вступление в чат %d не удалось (%v), создаём новый", lobbyChatDesiredID, jerr)
	} else {
		log.Printf("lobby: lobbyChatDesiredID=0 — создаём новый чат при каждом запуске")
	}
	newID, cerr := createLobbyChat(token, lobbyChatCreateName)
	if cerr != nil {
		return 0, fmt.Errorf("lobby create: %w", cerr)
	}
	st, jerr := joinLobbyChat(token, userID, newID)
	if jerr != nil || st != http.StatusOK {
		if jerr == nil {
			jerr = fmt.Errorf("HTTP %d", st)
		}
		return 0, fmt.Errorf("lobby join new chat %d: %w", newID, jerr)
	}
	log.Printf("lobby: используется chat_id=%d — пропишите lobbyChatDesiredID=%d у всех клиентов для общего лобби", newID, newID)
	return newID, nil
}

func fetchLobbyHistory(token string, chatID int64) ([]chatLine, error) {
	u := fmt.Sprintf("%s/chat/messages?chat_id=%d&limit=50", authBaseURL, chatID)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("messages %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	var out struct {
		Messages []struct {
			ID       int64  `json:"id"`
			SenderID int64  `json:"sender_id"`
			Text     string `json:"text"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	// API отдаёт от новых к старым; для экрана — хронология снизу вверх.
	lines := make([]chatLine, 0, len(out.Messages))
	for i := len(out.Messages) - 1; i >= 0; i-- {
		m := out.Messages[i]
		lines = append(lines, chatLine{id: m.ID, senderID: m.SenderID, text: m.Text})
	}
	return lines, nil
}

func postLobbySend(token string, chatID, senderID int64, text string) error {
	body, err := json.Marshal(map[string]any{
		"chat_id":   chatID,
		"sender_id": senderID,
		"text":      text,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, authBaseURL+"/chat/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return nil
}
