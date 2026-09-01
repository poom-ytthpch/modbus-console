package modbus

import (
	"errors"
	"fmt"
	"sync"
)

type banks struct {
	holding  map[uint16]uint16
	input    map[uint16]uint16
	coil     map[uint16]uint16
	discrete map[uint16]uint16
}

type RelaySpec struct {
	SlaveID      uint8
	ChannelCount uint16
	CoilStart    uint16
	HoldingStart uint16
	CoilOn       uint16
	CoilOff      uint16
	HoldingOn    uint16
	HoldingOff   uint16
}

type Store struct {
	mu     sync.RWMutex
	slaves map[uint8]*banks
	Strict bool
	relay  *RelaySpec
}

func NewStore() *Store {
	return &Store{slaves: make(map[uint8]*banks)}
}

func (s *Store) ensure(slave uint8) *banks {
	b := s.slaves[slave]
	if b == nil {
		b = &banks{holding: map[uint16]uint16{}, input: map[uint16]uint16{}, coil: map[uint16]uint16{}, discrete: map[uint16]uint16{}}
		s.slaves[slave] = b
	}
	return b
}

func bankFor(b *banks, table Table) (map[uint16]uint16, error) {
	switch table {
	case Holding:
		return b.holding, nil
	case Input:
		return b.input, nil
	case Coil:
		return b.coil, nil
	case Discrete:
		return b.discrete, nil
	default:
		return nil, fmt.Errorf("invalid table %q", table)
	}
}

func (s *Store) Read(table Table, slave uint8, address, quantity uint16) ([]uint16, error) {
	if slave < 1 || slave > 247 || quantity < 1 {
		return nil, errors.New("invalid slave/quantity")
	}
	if uint32(address)+uint32(quantity) > 65536 {
		return nil, errors.New("register range exceeds 65535")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	b := s.slaves[slave]
	if b == nil {
		if s.Strict {
			return nil, errors.New("slave/register range not mapped")
		}
		return make([]uint16, quantity), nil
	}
	bank, err := bankFor(b, table)
	if err != nil {
		return nil, err
	}
	out := make([]uint16, quantity)
	for i := uint16(0); i < quantity; i++ {
		value, ok := bank[address+i]
		if !ok && s.Strict {
			return nil, errors.New("register range not mapped")
		}
		out[i] = value
	}
	return out, nil
}

func (s *Store) Set(table Table, slave uint8, address uint16, values []uint16) error {
	if slave < 1 || slave > 247 || len(values) == 0 {
		return errors.New("invalid slave/values")
	}
	if uint32(address)+uint32(len(values)) > 65536 {
		return errors.New("register range exceeds 65535")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.ensure(slave)
	bank, err := bankFor(b, table)
	if err != nil {
		return err
	}
	for i, value := range values {
		if table == Coil || table == Discrete {
			if value > 1 {
				return errors.New("coil/discrete values must be 0 or 1")
			}
		}
		bank[address+uint16(i)] = value
	}
	return nil
}

func (s *Store) Write(table Table, slave uint8, address uint16, values []uint16) error {
	if table != Holding && table != Coil {
		return errors.New("only holding/coil are writable")
	}
	if len(values) == 0 {
		return errors.New("values required")
	}
	if uint32(address)+uint32(len(values)) > 65536 {
		return errors.New("register range exceeds 65535")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.ensure(slave)
	if s.relay != nil && s.relay.SlaveID == slave {
		for i, raw := range values {
			addr := address + uint16(i)
			if err := s.writeRelayLocked(b, table, addr, raw); err != nil {
				if s.Strict {
					return err
				}
			}
		}
		return nil
	}
	bank, _ := bankFor(b, table)
	for i, value := range values {
		if table == Coil && value > 1 {
			return errors.New("coil values must be 0 or 1")
		}
		bank[address+uint16(i)] = value
	}
	return nil
}

func (s *Store) EnableRelay(spec RelaySpec) error {
	if spec.SlaveID < 1 || spec.SlaveID > 247 || spec.ChannelCount < 1 || spec.ChannelCount > 64 {
		return errors.New("invalid relay spec")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.relay = &spec
	b := s.ensure(spec.SlaveID)
	for i := uint16(0); i < spec.ChannelCount; i++ {
		b.coil[spec.CoilStart+i] = spec.CoilOff
		b.holding[spec.HoldingStart+i] = spec.HoldingOff
	}
	return nil
}

func (s *Store) writeRelayLocked(b *banks, table Table, address, raw uint16) error {
	r := s.relay
	if r == nil {
		return errors.New("relay not configured")
	}
	var index uint16
	var on bool
	switch table {
	case Coil:
		if address < r.CoilStart || address >= r.CoilStart+r.ChannelCount {
			return errors.New("relay coil address out of range")
		}
		if raw != r.CoilOn && raw != r.CoilOff {
			return errors.New("invalid relay coil value")
		}
		index = address - r.CoilStart
		on = raw == r.CoilOn
	case Holding:
		if address < r.HoldingStart || address >= r.HoldingStart+r.ChannelCount {
			return errors.New("relay holding address out of range")
		}
		if raw != r.HoldingOn && raw != r.HoldingOff {
			return errors.New("invalid relay holding value")
		}
		index = address - r.HoldingStart
		on = raw == r.HoldingOn
	default:
		return errors.New("invalid relay table")
	}
	if on {
		b.coil[r.CoilStart+index] = r.CoilOn
		b.holding[r.HoldingStart+index] = r.HoldingOn
	} else {
		b.coil[r.CoilStart+index] = r.CoilOff
		b.holding[r.HoldingStart+index] = r.HoldingOff
	}
	return nil
}

func SeedSWS(store *Store) error {
	return ResetSWSLab(store)
}
