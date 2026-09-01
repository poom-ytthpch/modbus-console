package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modbus-console/modbus-console/core/internal/modbus"
)

func hardeningHandler(t *testing.T) http.Handler {
	t.Helper()
	store := modbus.NewStore()
	if err := modbus.SeedSWS(store); err != nil {
		t.Fatal(err)
	}
	slave := modbus.NewServer(store)
	addr, err := slave.Start("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = slave.Close() })
	return New(Config{
		Token:          "0123456789abcdef0123456789abcdef",
		AllowedOrigins: []string{"https://console.example"},
		Version:        "test", ModbusAddress: addr, Store: store, Slave: slave, RTUMaster: modbus.NewRTUMaster(), RTUSlave: modbus.NewRTUSlave(store),
	}).Handler()
}

func authRequest(method, target string, body *bytes.Reader) *http.Request {
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, body)
	}
	req.Header.Set("Authorization", "Bearer 0123456789abcdef0123456789abcdef")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestRawRegisterAPIEnforcesProtocolBounds(t *testing.T) {
	handler := hardeningHandler(t)
	cases := []string{
		"/api/v1/slaves/5/registers?table=holding&address=0&quantity=126",
		"/api/v1/slaves/5/registers?table=coil&address=0&quantity=2001",
		"/api/v1/slaves/5/registers?table=holding&address=65535&quantity=2",
	}
	for _, target := range cases {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, authRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d body=%s", target, recorder.Code, recorder.Body.String())
		}
	}
}

func TestRawRegisterWriteEnforcesSizeAndRange(t *testing.T) {
	handler := hardeningHandler(t)
	bodies := []map[string]any{
		{"table": "holding", "address": 0, "values": []uint16{}},
		{"table": "holding", "address": 65535, "values": []uint16{1, 2}},
		{"table": "holding", "address": 0, "values": make([]uint16, 124)},
		{"table": "coil", "address": 0, "values": make([]uint16, 1969)},
	}
	for _, body := range bodies {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, authRequest(http.MethodPatch, "/api/v1/slaves/5/registers", bytes.NewReader(raw)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
		}
	}
}

func TestJSONDecoderRejectsMultipleDocuments(t *testing.T) {
	handler := hardeningHandler(t)
	payload := strings.NewReader(`{"host":"127.0.0.1","port":1502,"slaveId":5,"functionCode":3,"address":1,"quantity":1} {"extra":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/modbus/request", payload)
	req.Header.Set("Authorization", "Bearer 0123456789abcdef0123456789abcdef")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for multiple JSON documents, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}
