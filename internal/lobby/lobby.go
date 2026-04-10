package lobby

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"client/internal/config"
)

// DesiredChatID — ожидаемый id группового чата лобби.
// Если вступить не удаётся, клиент создаёт новый чат (POST /chat/create).
// 0 — каждый запуск создаёт отдельный чат (отладка).
const DesiredChatID int64 = 1

const createChatName = "Lobby"

const (
	MaxChatLines     = 120
	MaxDraftRunes    = 512
	SubscribeChanCap = 256
)

// SubscribeMessage — push с /ws/subscribe (не envelope игры).
type SubscribeMessage struct {
	ID        int64     `json:"id"`
	ChatID    int64     `json:"chat_id"`
	SenderID  int64     `json:"sender_id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type Line struct {
	ID       int64
	SenderID int64
	Text     string
}

func JoinChat(token string, userID, chatID int64) (status int, err error) {
	body, err := json.Marshal(map[string]int64{
		"chat_id": chatID,
		"user_id": userID,
	})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, config.HTTPBase()+"/chat/members", bytes.NewReader(body))
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

func createChat(token, name string) (chatID int64, err error) {
	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, config.HTTPBase()+"/chat/create", bytes.NewReader(body))
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

// EnsureChat вступает в DesiredChatID или создаёт групповой чат и вступает в него.
func EnsureChat(token string, userID int64) (chatID int64, err error) {
	if DesiredChatID > 0 {
		st, jerr := JoinChat(token, userID, DesiredChatID)
		if jerr == nil && st == http.StatusOK {
			return DesiredChatID, nil
		}
		if st == http.StatusUnauthorized {
			return 0, fmt.Errorf("lobby join: %w", jerr)
		}
		log.Printf("lobby: вступление в чат %d не удалось (%v), создаём новый", DesiredChatID, jerr)
	} else {
		log.Printf("lobby: DesiredChatID=0 — новый чат при каждом запуске")
	}
	newID, cerr := createChat(token, createChatName)
	if cerr != nil {
		return 0, fmt.Errorf("lobby create: %w", cerr)
	}
	st, jerr := JoinChat(token, userID, newID)
	if jerr != nil || st != http.StatusOK {
		if jerr == nil {
			jerr = fmt.Errorf("HTTP %d", st)
		}
		return 0, fmt.Errorf("lobby join new chat %d: %w", newID, jerr)
	}
	log.Printf("lobby: chat_id=%d — пропишите lobby.DesiredChatID=%d у всех клиентов для общего лобби", newID, newID)
	return newID, nil
}

func FetchHistory(token string, chatID int64) ([]Line, error) {
	u := fmt.Sprintf("%s/chat/messages?chat_id=%d&limit=50", config.HTTPBase(), chatID)
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
	lines := make([]Line, 0, len(out.Messages))
	for i := len(out.Messages) - 1; i >= 0; i-- {
		m := out.Messages[i]
		lines = append(lines, Line{ID: m.ID, SenderID: m.SenderID, Text: m.Text})
	}
	return lines, nil
}

func SendLine(token string, chatID, senderID int64, text string) error {
	body, err := json.Marshal(map[string]any{
		"chat_id":   chatID,
		"sender_id": senderID,
		"text":      text,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, config.HTTPBase()+"/chat/send", bytes.NewReader(body))
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
