package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

func UserIDFromAccessJWT(access string) (int64, error) {
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
