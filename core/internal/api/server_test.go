package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modbus-console/modbus-console/core/internal/modbus"
)

func TestControlAPIRequiresPairingTokenAndOriginAllowList(t *testing.T) {
	store := modbus.NewStore()
	if err := modbus.SeedSWS(store); err != nil {
		t.Fatal(err)
	}
	slave := modbus.NewServer(store)
	addr, err := slave.Start("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = slave.Close() }()

	handler := New(Config{Token: "test-token-that-is-long-enough", AllowedOrigins: []string{"https://console.example"}, Version: "test", ModbusAddress: addr, Store: store, Slave: slave, RTUMaster: modbus.NewRTUMaster(), RTUSlave: modbus.NewRTUSlave(store)}).Handler()

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/engine", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorized.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/engine", nil)
	req.Header.Set("Authorization", "Bearer test-token-that-is-long-enough")
	req.Header.Set("Origin", "https://console.example")
	ok := httptest.NewRecorder()
	handler.ServeHTTP(ok, req)
	if ok.Code != http.StatusOK || ok.Header().Get("Access-Control-Allow-Origin") != "https://console.example" {
		t.Fatalf("authorized request failed code=%d origin=%q", ok.Code, ok.Header().Get("Access-Control-Allow-Origin"))
	}

	blockedReq := httptest.NewRequest(http.MethodGet, "/api/v1/engine", nil)
	blockedReq.Header.Set("Authorization", "Bearer test-token-that-is-long-enough")
	blockedReq.Header.Set("Origin", "https://evil.example")
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, blockedReq)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("expected origin 403, got %d", blocked.Code)
	}
}
