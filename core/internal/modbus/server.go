package modbus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type ServerStats struct {
	Clients  int64  `json:"clients"`
	Requests uint64 `json:"requests"`
	Errors   uint64 `json:"errors"`
}

type Server struct {
	store    *Store
	listener net.Listener
	clients  atomic.Int64
	requests atomic.Uint64
	errors   atomic.Uint64
	wg       sync.WaitGroup
	closed   chan struct{}
}

func NewServer(store *Store) *Server {
	return &Server{store: store, closed: make(chan struct{})}
}

func (s *Server) Start(address string) (string, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return "", err
	}
	s.listener = listener
	s.wg.Add(1)
	go s.acceptLoop()
	return listener.Addr().String(), nil
}

func (s *Server) Close() error {
	select {
	case <-s.closed:
		return nil
	default:
		close(s.closed)
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.wg.Wait()
	return nil
}

func (s *Server) Stats() ServerStats {
	return ServerStats{Clients: s.clients.Load(), Requests: s.requests.Load(), Errors: s.errors.Load()}
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return
			default:
				s.errors.Add(1)
				continue
			}
		}
		s.clients.Add(1)
		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer s.wg.Done()
	defer s.clients.Add(-1)
	defer func() { _ = conn.Close() }()
	for {
		if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
			return
		}
		header := make([]byte, 7)
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		length := binary.BigEndian.Uint16(header[4:6])
		if length < 2 || length > 254 || binary.BigEndian.Uint16(header[2:4]) != 0 {
			s.errors.Add(1)
			return
		}
		pdu := make([]byte, int(length)-1)
		if _, err := io.ReadFull(conn, pdu); err != nil {
			return
		}
		s.requests.Add(1)
		response, err := s.dispatch(header[6], pdu)
		if err != nil {
			s.errors.Add(1)
		}
		frame := make([]byte, 7+len(response))
		copy(frame[0:4], header[0:4])
		binary.BigEndian.PutUint16(frame[4:6], uint16(len(response)+1))
		frame[6] = header[6]
		copy(frame[7:], response)
		if _, writeErr := conn.Write(frame); writeErr != nil {
			return
		}
	}
}

func exceptionPDU(fc FunctionCode, code byte) []byte { return []byte{byte(fc) | 0x80, code} }

func (s *Server) dispatch(slave uint8, pdu []byte) ([]byte, error) {
	if len(pdu) < 1 {
		return exceptionPDU(0, 3), errors.New("empty pdu")
	}
	fc := FunctionCode(pdu[0])
	switch fc {
	case 1, 2, 3, 4:
		if len(pdu) < 5 {
			return exceptionPDU(fc, 3), errors.New("truncated read pdu")
		}
		address := binary.BigEndian.Uint16(pdu[1:3])
		quantity := binary.BigEndian.Uint16(pdu[3:5])
		max := uint16(125)
		if fc <= 2 {
			max = 2000
		}
		if quantity < 1 || quantity > max {
			return exceptionPDU(fc, 3), errors.New("invalid read quantity")
		}
		table := map[FunctionCode]Table{1: Coil, 2: Discrete, 3: Holding, 4: Input}[fc]
		values, err := s.store.Read(table, slave, address, quantity)
		if err != nil {
			return exceptionPDU(fc, 2), err
		}
		if fc <= 2 {
			byteCount := int((quantity + 7) / 8)
			out := make([]byte, 2+byteCount)
			out[0], out[1] = byte(fc), byte(byteCount)
			for i, value := range values {
				if value != 0 {
					out[2+i/8] |= 1 << (i % 8)
				}
			}
			return out, nil
		}
		out := make([]byte, 2+len(values)*2)
		out[0], out[1] = byte(fc), byte(len(values)*2)
		for i, value := range values {
			binary.BigEndian.PutUint16(out[2+i*2:4+i*2], value)
		}
		return out, nil
	case 5:
		if len(pdu) < 5 {
			return exceptionPDU(fc, 3), errors.New("truncated FC05")
		}
		address := binary.BigEndian.Uint16(pdu[1:3])
		raw := binary.BigEndian.Uint16(pdu[3:5])
		if raw != 0x0000 && raw != 0xFF00 {
			return exceptionPDU(fc, 3), errors.New("invalid FC05 value")
		}
		value := uint16(0)
		if raw == 0xFF00 {
			value = 1
		}
		if err := s.store.Write(Coil, slave, address, []uint16{value}); err != nil {
			return exceptionPDU(fc, 2), err
		}
		return append([]byte(nil), pdu[:5]...), nil
	case 6:
		if len(pdu) < 5 {
			return exceptionPDU(fc, 3), errors.New("truncated FC06")
		}
		address := binary.BigEndian.Uint16(pdu[1:3])
		value := binary.BigEndian.Uint16(pdu[3:5])
		if err := s.store.Write(Holding, slave, address, []uint16{value}); err != nil {
			return exceptionPDU(fc, 2), err
		}
		return append([]byte(nil), pdu[:5]...), nil
	case 15:
		if len(pdu) < 6 {
			return exceptionPDU(fc, 3), errors.New("truncated FC15")
		}
		address := binary.BigEndian.Uint16(pdu[1:3])
		quantity := binary.BigEndian.Uint16(pdu[3:5])
		byteCount := int(pdu[5])
		if quantity < 1 || quantity > 1968 || byteCount != int((quantity+7)/8) || len(pdu) < 6+byteCount {
			return exceptionPDU(fc, 3), errors.New("invalid FC15 payload")
		}
		values := make([]uint16, quantity)
		for i := range values {
			values[i] = uint16((pdu[6+i/8] >> (i % 8)) & 1)
		}
		if err := s.store.Write(Coil, slave, address, values); err != nil {
			return exceptionPDU(fc, 2), err
		}
		out := make([]byte, 5)
		out[0] = byte(fc)
		binary.BigEndian.PutUint16(out[1:3], address)
		binary.BigEndian.PutUint16(out[3:5], quantity)
		return out, nil
	case 16:
		if len(pdu) < 6 {
			return exceptionPDU(fc, 3), errors.New("truncated FC16")
		}
		address := binary.BigEndian.Uint16(pdu[1:3])
		quantity := binary.BigEndian.Uint16(pdu[3:5])
		byteCount := int(pdu[5])
		if quantity < 1 || quantity > 123 || byteCount != int(quantity)*2 || len(pdu) < 6+byteCount {
			return exceptionPDU(fc, 3), errors.New("invalid FC16 payload")
		}
		values := make([]uint16, quantity)
		for i := range values {
			values[i] = binary.BigEndian.Uint16(pdu[6+i*2 : 8+i*2])
		}
		if err := s.store.Write(Holding, slave, address, values); err != nil {
			return exceptionPDU(fc, 2), err
		}
		out := make([]byte, 5)
		out[0] = byte(fc)
		binary.BigEndian.PutUint16(out[1:3], address)
		binary.BigEndian.PutUint16(out[3:5], quantity)
		return out, nil
	default:
		return exceptionPDU(fc, 1), fmt.Errorf("unsupported function code %d", fc)
	}
}

// dispatchPDU reuses the exact TCP slave PDU semantics for RTU framing.
func dispatchPDU(store *Store, slave uint8, pdu []byte) ([]byte, error) {
	return (&Server{store: store}).dispatch(slave, pdu)
}
