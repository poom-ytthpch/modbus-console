package modbus

import "testing"

func TestRequestRejectsInvalidFC05Value(t *testing.T) {
	req := Request{Host: "127.0.0.1", Port: 502, SlaveID: 1, FunctionCode: 5, Address: 0, Value: 2}
	if err := req.Validate(); err == nil {
		t.Fatal("expected FC05 value 2 to be rejected")
	}
}

func TestStoreRejectsRangeOverflowAndEmptyWrite(t *testing.T) {
	store := NewStore()
	if _, err := store.Read(Holding, 1, 65535, 2); err == nil {
		t.Fatal("expected read overflow rejection")
	}
	if err := store.Set(Holding, 1, 65535, []uint16{1, 2}); err == nil {
		t.Fatal("expected set overflow rejection")
	}
	if err := store.Write(Holding, 1, 65535, []uint16{1, 2}); err == nil {
		t.Fatal("expected write overflow rejection")
	}
	if err := store.Write(Holding, 1, 0, nil); err == nil {
		t.Fatal("expected empty write rejection")
	}
}
