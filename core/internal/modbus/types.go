package modbus

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type FunctionCode uint8

type Table string

const (
	Coil     Table = "coil"
	Discrete Table = "discrete"
	Holding  Table = "holding"
	Input    Table = "input"
)

type Request struct {
	Host         string       `json:"host"`
	Port         int          `json:"port"`
	SlaveID      uint8        `json:"slaveId"`
	FunctionCode FunctionCode `json:"functionCode"`
	Address      uint16       `json:"address"`
	Quantity     uint16       `json:"quantity,omitempty"`
	Value        uint16       `json:"value,omitempty"`
	Values       []uint16     `json:"values,omitempty"`
	TimeoutMS    int          `json:"timeoutMs,omitempty"`
}

type Result struct {
	FunctionCode  FunctionCode `json:"functionCode"`
	Address       uint16       `json:"address"`
	Quantity      uint16       `json:"quantity"`
	Values        []uint16     `json:"values"`
	Hex           []string     `json:"hex"`
	Binary        []string     `json:"binary"`
	LatencyMS     int64        `json:"latencyMs"`
	RawRequestHex string       `json:"rawRequestHex"`
	RawReplyHex   string       `json:"rawResponseHex"`
}

func (r Request) Timeout() time.Duration {
	ms := r.TimeoutMS
	if ms <= 0 {
		ms = 1500
	}
	if ms > 60000 {
		ms = 60000
	}
	return time.Duration(ms) * time.Millisecond
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.Host) == "" {
		return errors.New("host required")
	}
	if r.Port < 1 || r.Port > 65535 {
		return errors.New("port must be 1..65535")
	}
	return r.ValidateProtocol()
}

// ValidateProtocol validates the PDU independently of its transport.
func (r Request) ValidateProtocol() error {
	if r.SlaveID < 1 || r.SlaveID > 247 {
		return errors.New("slaveId must be 1..247")
	}
	if !supportedFC(r.FunctionCode) {
		return fmt.Errorf("unsupported function code %d", r.FunctionCode)
	}
	switch r.FunctionCode {
	case 1, 2:
		if r.Quantity < 1 || r.Quantity > 2000 {
			return errors.New("quantity must be 1..2000 for FC01/FC02")
		}
	case 3, 4:
		if r.Quantity < 1 || r.Quantity > 125 {
			return errors.New("quantity must be 1..125 for FC03/FC04")
		}
	case 5:
		if r.Value > 1 {
			return errors.New("FC05 value must be 0 or 1")
		}
	case 15:
		if len(r.Values) < 1 || len(r.Values) > 1968 {
			return errors.New("FC15 values must contain 1..1968 coils")
		}
		for _, value := range r.Values {
			if value > 1 {
				return errors.New("FC15 values must be 0 or 1")
			}
		}
	case 16:
		if len(r.Values) < 1 || len(r.Values) > 123 {
			return errors.New("FC16 values must contain 1..123 registers")
		}
	}
	quantity := uint32(r.Quantity)
	switch r.FunctionCode {
	case 5, 6:
		quantity = 1
	case 15, 16:
		quantity = uint32(len(r.Values))
	}
	if uint32(r.Address)+quantity > 65536 {
		return errors.New("register range exceeds 65535")
	}
	return nil
}

func supportedFC(fc FunctionCode) bool {
	switch fc {
	case 1, 2, 3, 4, 5, 6, 15, 16:
		return true
	default:
		return false
	}
}

func spacedHex(data []byte) string {
	encoded := strings.ToUpper(hex.EncodeToString(data))
	parts := make([]string, 0, len(encoded)/2)
	for i := 0; i < len(encoded); i += 2 {
		parts = append(parts, encoded[i:i+2])
	}
	return strings.Join(parts, " ")
}
