package modbus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"go.bug.st/serial"
)

type SerialConfig struct {
	Port      string `json:"port"`
	BaudRate  int    `json:"baudRate"`
	DataBits  int    `json:"dataBits"`
	Parity    string `json:"parity"`
	StopBits  string `json:"stopBits"`
	TimeoutMS int    `json:"timeoutMs,omitempty"`
}

func (c SerialConfig) Timeout() time.Duration {
	if c.TimeoutMS <= 0 {
		return 1500 * time.Millisecond
	}
	if c.TimeoutMS > 60000 {
		return 60 * time.Second
	}
	return time.Duration(c.TimeoutMS) * time.Millisecond
}

func (c SerialConfig) mode() (*serial.Mode, error) {
	if c.Port == "" {
		return nil, errors.New("serial port required")
	}
	if c.BaudRate <= 0 {
		return nil, errors.New("baudRate must be positive")
	}
	if c.DataBits < 5 || c.DataBits > 8 {
		return nil, errors.New("dataBits must be 5..8")
	}
	var parity serial.Parity
	switch c.Parity {
	case "", "none":
		parity = serial.NoParity
	case "odd":
		parity = serial.OddParity
	case "even":
		parity = serial.EvenParity
	case "mark":
		parity = serial.MarkParity
	case "space":
		parity = serial.SpaceParity
	default:
		return nil, errors.New("parity must be none, odd, even, mark or space")
	}
	var stop serial.StopBits
	switch c.StopBits {
	case "", "1":
		stop = serial.OneStopBit
	case "1.5":
		stop = serial.OnePointFiveStopBits
	case "2":
		stop = serial.TwoStopBits
	default:
		return nil, errors.New("stopBits must be 1, 1.5 or 2")
	}
	return &serial.Mode{BaudRate: c.BaudRate, DataBits: c.DataBits, Parity: parity, StopBits: stop}, nil
}

func CRC16(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b)
		for range 8 {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

func appendCRC(frame []byte) []byte {
	crc := CRC16(frame)
	return append(frame, byte(crc), byte(crc>>8))
}

func verifyCRC(frame []byte) bool {
	if len(frame) < 4 {
		return false
	}
	got := binary.LittleEndian.Uint16(frame[len(frame)-2:])
	return got == CRC16(frame[:len(frame)-2])
}

func RTUSilentInterval(baud int) time.Duration {
	if baud <= 0 {
		baud = 9600
	}
	if baud > 19200 {
		return 1750 * time.Microsecond
	}
	return time.Duration(float64(time.Second) * 38.5 / float64(baud)) // 3.5 chars, conservative 11 bits/char.
}

type RTUMaster struct {
	mu     sync.Mutex
	port   serial.Port
	config SerialConfig
	lastIO time.Time
}

func NewRTUMaster() *RTUMaster { return &RTUMaster{} }

func (m *RTUMaster) Open(cfg SerialConfig) error {
	mode, err := cfg.mode()
	if err != nil {
		return err
	}
	p, err := serial.Open(cfg.Port, mode)
	if err != nil {
		return err
	}
	if err := p.SetReadTimeout(cfg.Timeout()); err != nil {
		_ = p.Close()
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.port != nil {
		_ = m.port.Close()
	}
	m.port, m.config, m.lastIO = p, cfg, time.Time{}
	return nil
}

func (m *RTUMaster) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.port == nil {
		return nil
	}
	err := m.port.Close()
	m.port = nil
	return err
}

func (m *RTUMaster) Config() (SerialConfig, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.config, m.port != nil
}

func (m *RTUMaster) Execute(req Request) (Result, error) {
	if err := req.ValidateProtocol(); err != nil {
		return Result{}, err
	}
	pdu, quantity, err := buildPDU(req)
	if err != nil {
		return Result{}, err
	}
	frame := appendCRC(append([]byte{req.SlaveID}, pdu...))
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.port == nil {
		return Result{}, errors.New("RTU master port is not open")
	}
	gap := RTUSilentInterval(m.config.BaudRate)
	if wait := gap - time.Since(m.lastIO); !m.lastIO.IsZero() && wait > 0 {
		time.Sleep(wait)
	}
	_ = m.port.ResetInputBuffer()
	started := time.Now()
	if _, err := m.port.Write(frame); err != nil {
		_ = m.port.Close()
		m.port = nil
		return Result{}, fmt.Errorf("serial write: %w", err)
	}
	if err := m.port.Drain(); err != nil {
		_ = m.port.Close()
		m.port = nil
		return Result{}, fmt.Errorf("serial drain: %w", err)
	}
	response, err := readRTUResponse(m.port, req, quantity, req.Timeout())
	m.lastIO = time.Now()
	if err != nil {
		_ = m.port.Close()
		m.port = nil
		return Result{}, err
	}
	values, resultQuantity, err := parsePDUResponse(req, quantity, response[1:len(response)-2])
	if err != nil {
		return Result{}, err
	}
	return makeResult(req, resultQuantity, values, started, frame, response), nil
}

func readExact(r io.Reader, buf []byte, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	offset := 0
	for offset < len(buf) {
		n, err := r.Read(buf[offset:])
		offset += n
		if err != nil {
			return offset, err
		}
		if offset == len(buf) {
			return offset, nil
		}
		if time.Now().After(deadline) {
			return offset, fmt.Errorf("serial read timeout after %s", timeout)
		}
		if n == 0 {
			time.Sleep(time.Millisecond)
		}
	}
	return offset, nil
}

func readRTUResponse(p io.Reader, req Request, quantity uint16, timeout time.Duration) ([]byte, error) {
	head := make([]byte, 2)
	if _, err := readExact(p, head, timeout); err != nil {
		return nil, fmt.Errorf("serial response header: %w", err)
	}
	if head[0] != req.SlaveID {
		return nil, errors.New("slave id mismatch")
	}
	fc := FunctionCode(head[1])
	var remaining int
	if fc&0x80 != 0 {
		remaining = 3
	} else {
		switch fc {
		case 1, 2, 3, 4:
			count := make([]byte, 1)
			if _, err := readExact(p, count, timeout); err != nil {
				return nil, err
			}
			remaining = int(count[0]) + 2
			tail := make([]byte, remaining)
			if _, err := readExact(p, tail, timeout); err != nil {
				return nil, err
			}
			frame := append(append(head, count...), tail...)
			if !verifyCRC(frame) {
				return nil, errors.New("RTU CRC mismatch")
			}
			return frame, nil
		case 5, 6, 15, 16:
			remaining = 6
		default:
			return nil, fmt.Errorf("unsupported RTU response FC%d", fc)
		}
	}
	tail := make([]byte, remaining)
	if _, err := readExact(p, tail, timeout); err != nil {
		return nil, err
	}
	frame := append(head, tail...)
	if !verifyCRC(frame) {
		return nil, errors.New("RTU CRC mismatch")
	}
	return frame, nil
}

func parsePDUResponse(req Request, quantity uint16, pdu []byte) ([]uint16, uint16, error) {
	if len(pdu) < 2 {
		return nil, 0, errors.New("truncated Modbus response")
	}
	fc := FunctionCode(pdu[0])
	if fc&0x80 != 0 {
		return nil, 0, fmt.Errorf("modbus exception %d", pdu[1])
	}
	if fc != req.FunctionCode {
		return nil, 0, errors.New("function code mismatch")
	}
	switch fc {
	case 1, 2:
		count := int(pdu[1])
		if len(pdu) < 2+count {
			return nil, 0, errors.New("truncated bit response")
		}
		values := make([]uint16, quantity)
		for i := range values {
			values[i] = uint16((pdu[2+i/8] >> (i % 8)) & 1)
		}
		return values, quantity, nil
	case 3, 4:
		count := int(pdu[1])
		if count != int(quantity)*2 || len(pdu) < 2+count {
			return nil, 0, errors.New("unexpected register byte count")
		}
		values := make([]uint16, quantity)
		for i := range values {
			values[i] = binary.BigEndian.Uint16(pdu[2+i*2 : 4+i*2])
		}
		return values, quantity, nil
	case 5:
		if len(pdu) < 5 || binary.BigEndian.Uint16(pdu[1:3]) != req.Address {
			return nil, 0, errors.New("FC05 echo mismatch")
		}
		raw := binary.BigEndian.Uint16(pdu[3:5])
		if raw == 0xFF00 {
			return []uint16{1}, 1, nil
		}
		if raw == 0 {
			return []uint16{0}, 1, nil
		}
		return nil, 0, errors.New("invalid FC05 echo value")
	case 6:
		if len(pdu) < 5 || binary.BigEndian.Uint16(pdu[1:3]) != req.Address {
			return nil, 0, errors.New("FC06 echo mismatch")
		}
		return []uint16{binary.BigEndian.Uint16(pdu[3:5])}, 1, nil
	case 15, 16:
		if len(pdu) < 5 {
			return nil, 0, errors.New("truncated multi-write response")
		}
		if binary.BigEndian.Uint16(pdu[1:3]) != req.Address || binary.BigEndian.Uint16(pdu[3:5]) != quantity {
			return nil, 0, errors.New("multi-write echo mismatch")
		}
		return append([]uint16(nil), req.Values...), quantity, nil
	}
	return nil, 0, errors.New("unsupported response")
}

func makeResult(req Request, quantity uint16, values []uint16, started time.Time, rawReq, rawResp []byte) Result {
	hexValues := make([]string, len(values))
	binaryValues := make([]string, len(values))
	for i, v := range values {
		hexValues[i] = fmt.Sprintf("0x%04X", v)
		width := 16
		if req.FunctionCode == 1 || req.FunctionCode == 2 || req.FunctionCode == 5 || req.FunctionCode == 15 {
			width = 1
		}
		binaryValues[i] = fmt.Sprintf("%0*b", width, v)
	}
	return Result{FunctionCode: req.FunctionCode, Address: req.Address, Quantity: quantity, Values: values, Hex: hexValues, Binary: binaryValues, LatencyMS: time.Since(started).Milliseconds(), RawRequestHex: spacedHex(rawReq), RawReplyHex: spacedHex(rawResp)}
}

type RTUSlaveStats struct {
	Requests uint64 `json:"requests"`
	Errors   uint64 `json:"errors"`
}
type RTUSlave struct {
	mu       sync.Mutex
	port     serial.Port
	config   SerialConfig
	store    *Store
	requests atomic.Uint64
	errors   atomic.Uint64
	stop     chan struct{}
	wg       sync.WaitGroup
}

func NewRTUSlave(store *Store) *RTUSlave { return &RTUSlave{store: store} }
func (s *RTUSlave) Start(cfg SerialConfig) error {
	mode, err := cfg.mode()
	if err != nil {
		return err
	}
	p, err := serial.Open(cfg.Port, mode)
	if err != nil {
		return err
	}
	_ = p.SetReadTimeout(100 * time.Millisecond)
	s.mu.Lock()
	if s.port != nil {
		s.mu.Unlock()
		_ = p.Close()
		return errors.New("RTU slave already running")
	}
	s.port = p
	s.config = cfg
	s.stop = make(chan struct{})
	s.wg.Add(1)
	s.mu.Unlock()
	go s.loop()
	return nil
}
func (s *RTUSlave) Close() error {
	s.mu.Lock()
	if s.port == nil {
		s.mu.Unlock()
		return nil
	}
	close(s.stop)
	p := s.port
	s.port = nil
	s.mu.Unlock()
	_ = p.Close()
	s.wg.Wait()
	return nil
}
func (s *RTUSlave) Status() (SerialConfig, bool, RTUSlaveStats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config, s.port != nil, RTUSlaveStats{Requests: s.requests.Load(), Errors: s.errors.Load()}
}
func (s *RTUSlave) loop() {
	defer s.wg.Done()
	for {
		s.mu.Lock()
		p, stop := s.port, s.stop
		s.mu.Unlock()
		if p == nil {
			return
		}
		select {
		case <-stop:
			return
		default:
		}
		frame, err := readRTURequest(p, 150*time.Millisecond)
		if err != nil {
			// Read timeouts are expected while an idle slave waits for a master.
			continue
		}
		if !verifyCRC(frame) {
			s.errors.Add(1)
			continue
		}
		slave := frame[0]
		if slave == 0 { // Broadcast writes are intentionally not supported yet.
			s.errors.Add(1)
			continue
		}
		pdu := frame[1 : len(frame)-2]
		s.requests.Add(1)
		resp, dispatchErr := dispatchPDU(s.store, slave, pdu)
		if dispatchErr != nil {
			s.errors.Add(1)
		}
		out := appendCRC(append([]byte{slave}, resp...))
		time.Sleep(RTUSilentInterval(s.config.BaudRate))
		if _, err = p.Write(out); err != nil {
			s.errors.Add(1)
			return
		}
		if err = p.Drain(); err != nil {
			s.errors.Add(1)
			return
		}
	}
}

func readRTURequest(p io.Reader, timeout time.Duration) ([]byte, error) {
	head := make([]byte, 2)
	if _, err := readExact(p, head, timeout); err != nil {
		return nil, err
	}
	fc := FunctionCode(head[1])
	var tail []byte
	switch fc {
	case 1, 2, 3, 4, 5, 6:
		tail = make([]byte, 6) // address + quantity/value + CRC
	case 15, 16:
		fixed := make([]byte, 5) // address + quantity + byteCount
		if _, err := readExact(p, fixed, timeout); err != nil {
			return nil, err
		}
		count := int(fixed[4])
		dataAndCRC := make([]byte, count+2)
		if _, err := readExact(p, dataAndCRC, timeout); err != nil {
			return nil, err
		}
		return append(append(head, fixed...), dataAndCRC...), nil
	default:
		return nil, fmt.Errorf("unsupported RTU request FC%d", fc)
	}
	if _, err := readExact(p, tail, timeout); err != nil {
		return nil, err
	}
	return append(head, tail...), nil
}
