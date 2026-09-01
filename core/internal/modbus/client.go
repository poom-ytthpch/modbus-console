package modbus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"time"
)

var transactionID atomic.Uint32

func ExecuteTCP(req Request) (Result, error) {
	if err := req.Validate(); err != nil {
		return Result{}, err
	}
	pdu, quantity, err := buildPDU(req)
	if err != nil {
		return Result{}, err
	}
	tx := uint16(transactionID.Add(1))
	frame := make([]byte, 7+len(pdu))
	binary.BigEndian.PutUint16(frame[0:2], tx)
	binary.BigEndian.PutUint16(frame[2:4], 0)
	binary.BigEndian.PutUint16(frame[4:6], uint16(len(pdu)+1))
	frame[6] = req.SlaveID
	copy(frame[7:], pdu)

	started := time.Now()
	response, err := exchange(req.Host, req.Port, frame, req.Timeout())
	if err != nil {
		return Result{}, err
	}
	values, resultQuantity, err := parseResponse(req, tx, quantity, response)
	if err != nil {
		return Result{}, err
	}
	hexValues := make([]string, len(values))
	binaryValues := make([]string, len(values))
	for i, value := range values {
		hexValues[i] = fmt.Sprintf("0x%04X", value)
		width := 16
		if req.FunctionCode == 1 || req.FunctionCode == 2 || req.FunctionCode == 5 || req.FunctionCode == 15 {
			width = 1
		}
		binaryValues[i] = fmt.Sprintf("%0*b", width, value)
	}
	return Result{
		FunctionCode:  req.FunctionCode,
		Address:       req.Address,
		Quantity:      resultQuantity,
		Values:        values,
		Hex:           hexValues,
		Binary:        binaryValues,
		LatencyMS:     time.Since(started).Milliseconds(),
		RawRequestHex: spacedHex(frame),
		RawReplyHex:   spacedHex(response),
	}, nil
}

func buildPDU(req Request) ([]byte, uint16, error) {
	fc := req.FunctionCode
	switch fc {
	case 1, 2, 3, 4:
		pdu := make([]byte, 5)
		pdu[0] = byte(fc)
		binary.BigEndian.PutUint16(pdu[1:3], req.Address)
		binary.BigEndian.PutUint16(pdu[3:5], req.Quantity)
		return pdu, req.Quantity, nil
	case 5:
		pdu := make([]byte, 5)
		pdu[0] = byte(fc)
		binary.BigEndian.PutUint16(pdu[1:3], req.Address)
		if req.Value != 0 {
			binary.BigEndian.PutUint16(pdu[3:5], 0xFF00)
		}
		return pdu, 1, nil
	case 6:
		pdu := make([]byte, 5)
		pdu[0] = byte(fc)
		binary.BigEndian.PutUint16(pdu[1:3], req.Address)
		binary.BigEndian.PutUint16(pdu[3:5], req.Value)
		return pdu, 1, nil
	case 15:
		quantity := uint16(len(req.Values))
		byteCount := (len(req.Values) + 7) / 8
		pdu := make([]byte, 6+byteCount)
		pdu[0] = byte(fc)
		binary.BigEndian.PutUint16(pdu[1:3], req.Address)
		binary.BigEndian.PutUint16(pdu[3:5], quantity)
		pdu[5] = byte(byteCount)
		for i, value := range req.Values {
			if value != 0 {
				pdu[6+i/8] |= 1 << (i % 8)
			}
		}
		return pdu, quantity, nil
	case 16:
		quantity := uint16(len(req.Values))
		pdu := make([]byte, 6+len(req.Values)*2)
		pdu[0] = byte(fc)
		binary.BigEndian.PutUint16(pdu[1:3], req.Address)
		binary.BigEndian.PutUint16(pdu[3:5], quantity)
		pdu[5] = byte(len(req.Values) * 2)
		for i, value := range req.Values {
			binary.BigEndian.PutUint16(pdu[6+i*2:8+i*2], value)
		}
		return pdu, quantity, nil
	default:
		return nil, 0, fmt.Errorf("unsupported function code %d", fc)
	}
}

func exchange(host string, port int, frame []byte, timeout time.Duration) ([]byte, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), timeout)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	if _, err := conn.Write(frame); err != nil {
		return nil, err
	}
	header := make([]byte, 7)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint16(header[4:6])
	if length < 2 || length > 254 {
		return nil, fmt.Errorf("invalid MBAP length %d", length)
	}
	body := make([]byte, int(length)-1)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, err
	}
	return append(header, body...), nil
}

func parseResponse(req Request, tx, quantity uint16, frame []byte) ([]uint16, uint16, error) {
	if len(frame) < 9 {
		return nil, 0, errors.New("truncated Modbus response")
	}
	if binary.BigEndian.Uint16(frame[0:2]) != tx {
		return nil, 0, errors.New("transaction id mismatch")
	}
	if binary.BigEndian.Uint16(frame[2:4]) != 0 {
		return nil, 0, errors.New("invalid protocol id")
	}
	if frame[6] != req.SlaveID {
		return nil, 0, errors.New("slave id mismatch")
	}
	fc := FunctionCode(frame[7])
	if fc&0x80 != 0 {
		return nil, 0, fmt.Errorf("modbus exception %d", frame[8])
	}
	if fc != req.FunctionCode {
		return nil, 0, errors.New("function code mismatch")
	}
	switch fc {
	case 1, 2:
		byteCount := int(frame[8])
		if len(frame) < 9+byteCount {
			return nil, 0, errors.New("truncated bit response")
		}
		values := make([]uint16, quantity)
		for i := range values {
			values[i] = uint16((frame[9+i/8] >> (i % 8)) & 1)
		}
		return values, quantity, nil
	case 3, 4:
		byteCount := int(frame[8])
		if byteCount != int(quantity)*2 || len(frame) < 9+byteCount {
			return nil, 0, errors.New("unexpected register byte count")
		}
		values := make([]uint16, quantity)
		for i := range values {
			values[i] = binary.BigEndian.Uint16(frame[9+i*2 : 11+i*2])
		}
		return values, quantity, nil
	case 5:
		if len(frame) < 12 || binary.BigEndian.Uint16(frame[8:10]) != req.Address {
			return nil, 0, errors.New("FC05 echo mismatch")
		}
		raw := binary.BigEndian.Uint16(frame[10:12])
		if raw == 0xFF00 {
			return []uint16{1}, 1, nil
		}
		if raw == 0x0000 {
			return []uint16{0}, 1, nil
		}
		return nil, 0, errors.New("invalid FC05 echo value")
	case 6:
		if len(frame) < 12 || binary.BigEndian.Uint16(frame[8:10]) != req.Address {
			return nil, 0, errors.New("FC06 echo mismatch")
		}
		return []uint16{binary.BigEndian.Uint16(frame[10:12])}, 1, nil
	case 15, 16:
		if len(frame) < 12 {
			return nil, 0, errors.New("truncated multi-write response")
		}
		address := binary.BigEndian.Uint16(frame[8:10])
		written := binary.BigEndian.Uint16(frame[10:12])
		if address != req.Address || written != quantity {
			return nil, 0, errors.New("multi-write echo mismatch")
		}
		return append([]uint16(nil), req.Values...), written, nil
	default:
		return nil, 0, errors.New("unsupported response")
	}
}
