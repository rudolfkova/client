package characters

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"client/internal/config"
)

// ErrCharacterServiceUnconfigured — gateway вернул 503 character_service_unconfigured (игра без character_id).
var ErrCharacterServiceUnconfigured = errors.New("character_service_unconfigured")

// Summary — элемент списка GET /api/me/characters (без data).
type Summary struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	Description    string `json:"description"`
	SchemaVersion  int32  `json:"schema_version"`
	Version        int64  `json:"version"`
}

type listResponse struct {
	Characters []Summary `json:"characters"`
}

type gatewayErr struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ListMine вызывает GET /api/me/characters с Bearer access_token.
// limit по умолчанию 50 (сервер максимум 100); offset для пагинации.
func ListMine(accessToken string, limit, offset int) ([]Summary, error) {
	if limit <= 0 {
		limit = 50
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	reqURL := config.HTTPBase() + "/api/me/characters?" + q.Encode()

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusOK {
		var out listResponse
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("decode list: %w", err)
		}
		return out.Characters, nil
	}

	var ge gatewayErr
	_ = json.Unmarshal(raw, &ge)
	if resp.StatusCode == http.StatusServiceUnavailable && ge.Code == "character_service_unconfigured" {
		return nil, ErrCharacterServiceUnconfigured
	}
	msg := strings.TrimSpace(string(raw))
	if ge.Message != "" {
		msg = ge.Message
	} else if ge.Code != "" {
		msg = ge.Code
	}
	return nil, fmt.Errorf("GET /api/me/characters: %s — %s", resp.Status, msg)
}
