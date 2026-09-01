package modbus

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

func TestCRC16KnownVector(t *testing.T) {
	frame := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x0A}
	if got := CRC16(frame); got != 0xCDC5 {
		t.Fatalf("CRC=%04X want CDC5", got)
	}
	if !verifyCRC(appendCRC(frame)) {
		t.Fatal("appended CRC did not verify")
	}
}

func TestRTUSilentInterval(t *testing.T) {
	if got := RTUSilentInterval(9600); got < 4*time.Millisecond || got > 5*time.Millisecond {
		t.Fatalf("9600 gap=%v", got)
	}
	if got := RTUSilentInterval(115200); got != 1750*time.Microsecond {
		t.Fatalf("115200 gap=%v", got)
	}
}

func TestReadRTURequestFC16(t *testing.T) {
	frame := appendCRC([]byte{5, 16, 0, 1, 0, 2, 4, 0, 100, 0, 200})
	got, err := readRTURequest(bytes.NewReader(frame), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, frame) {
		t.Fatalf("frame=% X want % X", got, frame)
	}
}

func TestParseRTURegisterResponse(t *testing.T) {
	req := Request{SlaveID: 8, FunctionCode: 4, Address: 1, Quantity: 2}
	frame := appendCRC([]byte{8, 4, 4, 0x01, 0x1D, 0x02, 0xD0})
	got, err := readRTUResponse(bytes.NewReader(frame), req, 2, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	values, quantity, err := parsePDUResponse(req, 2, got[1:len(got)-2])
	if err != nil {
		t.Fatal(err)
	}
	if quantity != 2 || values[0] != 285 || values[1] != 720 {
		t.Fatalf("quantity=%d values=%v", quantity, values)
	}
}

func TestRTURejectsBadCRC(t *testing.T) {
	req := Request{SlaveID: 1, FunctionCode: 3, Address: 0, Quantity: 1}
	frame := []byte{1, 3, 2, 0, 42, 0, 0}
	if _, err := readRTUResponse(bytes.NewReader(frame), req, 1, time.Second); err == nil {
		t.Fatal("expected CRC error")
	}
}

func TestProtocolRangeValidation(t *testing.T) {
	req := Request{SlaveID: 1, FunctionCode: 16, Address: 65535, Values: []uint16{1, 2}}
	if err := req.ValidateProtocol(); err == nil {
		t.Fatal("expected address overflow")
	}
}

func TestRTUFC06EchoParser(t *testing.T) {
	req := Request{SlaveID: 2, FunctionCode: 6, Address: 7, Value: 0x1234}
	pdu := []byte{6, 0, 7, 0x12, 0x34}
	values, q, err := parsePDUResponse(req, 1, pdu)
	if err != nil {
		t.Fatal(err)
	}
	if q != 1 || len(values) != 1 || values[0] != 0x1234 {
		t.Fatalf("q=%d values=%v", q, values)
	}
}

func TestRTUSoftwareLoopFC01ToFC16(t *testing.T) {
	store := NewStore()
	if err := store.Set(Coil, 10, 0, []uint16{1, 0, 1, 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(Discrete, 10, 0, []uint16{0, 1, 0, 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(Holding, 10, 0, []uint16{100, 200, 300, 400}); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(Input, 10, 0, []uint16{500, 600, 700, 800}); err != nil {
		t.Fatal(err)
	}
	requests := []Request{
		{SlaveID: 10, FunctionCode: 1, Address: 0, Quantity: 4},
		{SlaveID: 10, FunctionCode: 2, Address: 0, Quantity: 4},
		{SlaveID: 10, FunctionCode: 3, Address: 0, Quantity: 2},
		{SlaveID: 10, FunctionCode: 4, Address: 0, Quantity: 2},
		{SlaveID: 10, FunctionCode: 5, Address: 1, Value: 1},
		{SlaveID: 10, FunctionCode: 6, Address: 1, Value: 222},
		{SlaveID: 10, FunctionCode: 15, Address: 0, Values: []uint16{0, 1, 1, 0}},
		{SlaveID: 10, FunctionCode: 16, Address: 0, Values: []uint16{11, 22, 33}},
	}
	for _, req := range requests {
		req := req
		t.Run(fmt.Sprintf("FC%02d", req.FunctionCode), func(t *testing.T) {
			if err := req.ValidateProtocol(); err != nil {
				t.Fatal(err)
			}
			pdu, quantity, err := buildPDU(req)
			if err != nil {
				t.Fatal(err)
			}
			wireReq := appendCRC(append([]byte{req.SlaveID}, pdu...))
			parsedReq, err := readRTURequest(bytes.NewReader(wireReq), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			respPDU, _ := dispatchPDU(store, parsedReq[0], parsedReq[1:len(parsedReq)-2])
			wireResp := appendCRC(append([]byte{req.SlaveID}, respPDU...))
			parsedResp, err := readRTUResponse(bytes.NewReader(wireResp), req, quantity, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := parsePDUResponse(req, quantity, parsedResp[1:len(parsedResp)-2]); err != nil {
				t.Fatal(err)
			}
		})
	}
}
