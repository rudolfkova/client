package config

import (
	"strings"
)

// gatewayHostPort — host:port gateway (HTTP и WebSocket без схемы), по умолчанию до SetGateway*.
var gatewayHostPort = "localhost:8080"

// SetGatewayHostPort задаёт host:port (без схемы и пути; можно ввести с http:// или ws://). Пустая строка — не менять.
func SetGatewayHostPort(hostPort string) {
	hostPort = normalizeGatewayHostPort(hostPort)
	if hostPort == "" {
		return
	}
	gatewayHostPort = hostPort
}

func normalizeGatewayHostPort(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	low := strings.ToLower(s)
	for _, p := range []string{"https://", "http://", "wss://", "ws://"} {
		if strings.HasPrefix(low, p) {
			s = s[len(p):]
			low = strings.ToLower(s)
			break
		}
	}
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// GatewayHostPort возвращает текущий host:port gateway.
func GatewayHostPort() string {
	return gatewayHostPort
}

// HTTPBase — базовый URL REST gateway.
func HTTPBase() string {
	return "http://" + gatewayHostPort
}
