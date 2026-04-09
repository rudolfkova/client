package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// lobbyChatID — id группового чата лобби (создайте чат на сервере и подставьте сюда).
const lobbyChatID int64 = 3

const (
	maxLobbyChatLines = 120
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

func joinLobbyChat(token string, userID, chatID int64) error {
	body, err := json.Marshal(map[string]int64{
		"chat_id": chatID,
		"user_id": userID,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, authBaseURL+"/chat/members", bytes.NewReader(body))
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
