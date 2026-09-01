package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/modbus-console/modbus-console/core/internal/modbus"
)

func TestSimulatorAdminMutatesReadOnlySensorAndModbusSlaveServesIt(t *testing.T) {
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

	const token = "simulator-test-token-0123456789abcdef"
	handler := New(Config{
		Token: token, AllowedOrigins: []string{"http://localhost:3000"}, Version: "test",
		ModbusAddress: addr, Store: store, Slave: slave,
		RTUMaster: modbus.NewRTUMaster(), RTUSlave: modbus.NewRTUSlave(store),
	}).Handler()

	profileReq := httptest.NewRequest(http.MethodGet, "/api/v1/simulator/profile", nil)
	profileReq.Header.Set("Authorization", "Bearer "+token)
	profileRes := httptest.NewRecorder()
	handler.ServeHTTP(profileRes, profileReq)
	if profileRes.Code != http.StatusOK {
		t.Fatalf("profile status=%d body=%s", profileRes.Code, profileRes.Body.String())
	}
	var profile modbus.SimulatorProfile
	if err := json.Unmarshal(profileRes.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.ID != "sws-lab-v1" || len(profile.Devices) != 8 {
		t.Fatalf("unexpected profile: %+v", profile)
	}
	foundFlowmeter := false
	for _, device := range profile.Devices {
		if device.Key == "flowmeter-15" && device.SlaveID == 15 && len(device.Ranges) == 1 && len(device.Ranges[0].Values) == 2 {
			foundFlowmeter = true
		}
	}
	if !foundFlowmeter {
		t.Fatal("flowmeter-15 profile missing")
	}

	// Input registers are read-only to Modbus clients, but the simulator admin
	// control plane must be able to make a virtual sensor change its reading.
	writeReq := httptest.NewRequest(http.MethodPatch, "/api/v1/simulator/registers", strings.NewReader(`{"slaveId":8,"table":"input","address":1,"values":[301,655]}`))
	writeReq.Header.Set("Authorization", "Bearer "+token)
	writeRes := httptest.NewRecorder()
	handler.ServeHTTP(writeRes, writeReq)
	if writeRes.Code != http.StatusOK {
		t.Fatalf("simulator write status=%d body=%s", writeRes.Code, writeRes.Body.String())
	}

	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	result, err := modbus.ExecuteTCP(modbus.Request{Host: host, Port: port, SlaveID: 8, FunctionCode: 4, Address: 1, Quantity: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Values) != 2 || result.Values[0] != 301 || result.Values[1] != 655 {
		t.Fatalf("virtual sensor did not serve mutated values: %v", result.Values)
	}

	resetReq := httptest.NewRequest(http.MethodPost, "/api/v1/simulator/reset", nil)
	resetReq.Header.Set("Authorization", "Bearer "+token)
	resetRes := httptest.NewRecorder()
	handler.ServeHTTP(resetRes, resetReq)
	if resetRes.Code != http.StatusOK {
		t.Fatalf("reset status=%d body=%s", resetRes.Code, resetRes.Body.String())
	}
	result, err = modbus.ExecuteTCP(modbus.Request{Host: host, Port: port, SlaveID: 8, FunctionCode: 4, Address: 1, Quantity: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Values[0] != 285 || result.Values[1] != 720 {
		t.Fatalf("reset values unexpected: %v", result.Values)
	}
}
