package config

// GatewayHostPort — host:port gateway (HTTP и WebSocket без схемы).
const GatewayHostPort = "localhost:8080"

// HTTPBase — базовый URL REST gateway.
func HTTPBase() string {
	return "http://" + GatewayHostPort
}
