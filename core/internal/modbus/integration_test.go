package modbus

import (
	"net"
	"strconv"
	"testing"
)

func startTestServer(t *testing.T) (*Store, *Server, string, int) {
	t.Helper()
	store := NewStore()
	if err := SeedSWS(store); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(Discrete, 20, 0, []uint16{1, 0, 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(Input, 20, 10, []uint16{111, 222}); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(Coil, 20, 0, []uint16{0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(Holding, 20, 0, []uint16{10, 20, 30, 40}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(store)
	addr, err := server.Start("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return store, server, host, port
}

func execute(t *testing.T, host string, port int, req Request) Result {
	t.Helper()
	req.Host = host
	req.Port = port
	result, err := ExecuteTCP(req)
	if err != nil {
		t.Fatalf("FC%d failed: %v", req.FunctionCode, err)
	}
	if result.RawRequestHex == "" || result.RawReplyHex == "" {
		t.Fatal("raw TX/RX evidence missing")
	}
	return result
}

func TestTCPMasterSlaveFC01ToFC16(t *testing.T) {
	_, server, host, port := startTestServer(t)

	if got := execute(t, host, port, Request{SlaveID: 20, FunctionCode: 1, Address: 0, Quantity: 4}).Values; len(got) != 4 || got[0] != 0 {
		t.Fatalf("FC01 unexpected: %v", got)
	}
	if got := execute(t, host, port, Request{SlaveID: 20, FunctionCode: 2, Address: 0, Quantity: 3}).Values; got[0] != 1 || got[2] != 1 {
		t.Fatalf("FC02 unexpected: %v", got)
	}
	if got := execute(t, host, port, Request{SlaveID: 20, FunctionCode: 3, Address: 0, Quantity: 4}).Values; got[0] != 10 || got[3] != 40 {
		t.Fatalf("FC03 unexpected: %v", got)
	}
	if got := execute(t, host, port, Request{SlaveID: 20, FunctionCode: 4, Address: 10, Quantity: 2}).Values; got[0] != 111 || got[1] != 222 {
		t.Fatalf("FC04 unexpected: %v", got)
	}
	if got := execute(t, host, port, Request{SlaveID: 20, FunctionCode: 5, Address: 1, Value: 1}).Values; got[0] != 1 {
		t.Fatalf("FC05 unexpected: %v", got)
	}
	if got := execute(t, host, port, Request{SlaveID: 20, FunctionCode: 6, Address: 1, Value: 999}).Values; got[0] != 999 {
		t.Fatalf("FC06 unexpected: %v", got)
	}
	execute(t, host, port, Request{SlaveID: 20, FunctionCode: 15, Address: 0, Values: []uint16{1, 0, 1, 1}})
	if got := execute(t, host, port, Request{SlaveID: 20, FunctionCode: 1, Address: 0, Quantity: 4}).Values; got[0] != 1 || got[2] != 1 || got[3] != 1 {
		t.Fatalf("FC15 readback unexpected: %v", got)
	}
	execute(t, host, port, Request{SlaveID: 20, FunctionCode: 16, Address: 0, Values: []uint16{101, 202, 303}})
	if got := execute(t, host, port, Request{SlaveID: 20, FunctionCode: 3, Address: 0, Quantity: 3}).Values; got[0] != 101 || got[2] != 303 {
		t.Fatalf("FC16 readback unexpected: %v", got)
	}

	stats := server.Stats()
	if stats.Requests < 10 || stats.Errors != 0 {
		t.Fatalf("server stats unexpected: %+v", stats)
	}
}

func TestSWSFlowmeter15Preset(t *testing.T) {
	_, _, host, port := startTestServer(t)
	got := execute(t, host, port, Request{SlaveID: 15, FunctionCode: 3, Address: 0, Quantity: 2}).Values
	if len(got) != 2 || got[0] != 125 || got[1] != 4582 {
		t.Fatalf("Flowmeter 15 preset unexpected: %v", got)
	}
}

func TestSWSFactoryChlorinePreset(t *testing.T) {
	_, _, host, port := startTestServer(t)
	got := execute(t, host, port, Request{SlaveID: 4, FunctionCode: 3, Address: 2, Quantity: 2}).Values
	if len(got) != 2 || got[0] != 0x4020 || got[1] != 0x0000 {
		t.Fatalf("CHLORINE preset unexpected: %v", got)
	}
}

func TestSWSRelayActiveLowAndHREGStaySynchronized(t *testing.T) {
	_, _, host, port := startTestServer(t)

	// Physical ON in SWS COIL mode is raw/logical 0.
	execute(t, host, port, Request{SlaveID: 5, FunctionCode: 5, Address: 0, Value: 0})
	if got := execute(t, host, port, Request{SlaveID: 5, FunctionCode: 1, Address: 0, Quantity: 1}).Values[0]; got != 0 {
		t.Fatalf("active-low coil ON readback = %d", got)
	}
	if got := execute(t, host, port, Request{SlaveID: 5, FunctionCode: 3, Address: 1, Quantity: 1}).Values[0]; got != 256 {
		t.Fatalf("HREG ON readback = %d", got)
	}

	// Physical OFF via HREG must synchronize the COIL view to raw/logical 1.
	execute(t, host, port, Request{SlaveID: 5, FunctionCode: 6, Address: 1, Value: 512})
	if got := execute(t, host, port, Request{SlaveID: 5, FunctionCode: 1, Address: 0, Quantity: 1}).Values[0]; got != 1 {
		t.Fatalf("coil OFF readback = %d", got)
	}
}
