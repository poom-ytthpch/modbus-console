package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/modbus-console/modbus-console/core/internal/modbus"
	"github.com/modbus-console/modbus-console/core/internal/serialports"
)

type Config struct {
	Token          string
	AllowedOrigins []string
	Version        string
	ModbusAddress  string
	Store          *modbus.Store
	Slave          *modbus.Server
	RTUMaster      *modbus.RTUMaster
	RTUSlave       *modbus.RTUSlave
}

type Server struct {
	cfg     Config
	allowed map[string]struct{}
}

func New(cfg Config) *Server {
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return &Server{cfg: cfg, allowed: allowed}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /api/v1/engine", s.authorized(s.engine))
	mux.HandleFunc("GET /api/v1/serial/ports", s.authorized(s.serialPorts))
	mux.HandleFunc("POST /api/v1/rtu/master/open", s.authorized(s.openRTUMaster))
	mux.HandleFunc("DELETE /api/v1/rtu/master", s.authorized(s.closeRTUMaster))
	mux.HandleFunc("POST /api/v1/rtu/request", s.authorized(s.rtuRequest))
	mux.HandleFunc("POST /api/v1/rtu/slave/start", s.authorized(s.startRTUSlave))
	mux.HandleFunc("DELETE /api/v1/rtu/slave", s.authorized(s.stopRTUSlave))
	mux.HandleFunc("GET /api/v1/rtu/status", s.authorized(s.rtuStatus))
	mux.HandleFunc("POST /api/v1/modbus/request", s.authorized(s.modbusRequest))
	mux.HandleFunc("GET /api/v1/simulator/profile", s.authorized(s.simulatorProfile))
	mux.HandleFunc("POST /api/v1/simulator/reset", s.authorized(s.resetSimulator))
	mux.HandleFunc("PATCH /api/v1/simulator/registers", s.authorized(s.writeSimulatorRegisters))
	mux.HandleFunc("GET /api/v1/slaves/{slaveId}/registers", s.authorized(s.readRegisters))
	mux.HandleFunc("PATCH /api/v1/slaves/{slaveId}/registers", s.authorized(s.writeRegisters))
	return s.cors(mux)
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := s.allowed[origin]; !ok {
				writeError(w, http.StatusForbidden, "origin not allowed")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		const prefix = "Bearer "
		candidate := ""
		if strings.HasPrefix(header, prefix) {
			candidate = strings.TrimSpace(strings.TrimPrefix(header, prefix))
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(s.cfg.Token)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.cfg.Version})
}

func (s *Server) engine(w http.ResponseWriter, _ *http.Request) {
	ports, _ := serialports.List()
	_, masterOpen := s.cfg.RTUMaster.Config()
	_, slaveOpen, rtuStats := s.cfg.RTUSlave.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":        s.cfg.Version,
		"platform":       runtime.GOOS,
		"arch":           runtime.GOARCH,
		"modbusTcpSlave": map[string]any{"address": s.cfg.ModbusAddress, "stats": s.cfg.Slave.Stats()},
		"capabilities": map[string]any{
			"tcpMaster":       true,
			"tcpSlave":        true,
			"serialDiscovery": true,
			"rtuMaster":       true,
			"rtuSlave":        true,
		},
		"modbusRtu":       map[string]any{"masterOpen": masterOpen, "slaveOpen": slaveOpen, "stats": rtuStats},
		"serialPortCount": len(ports),
	})
}

func (s *Server) serialPorts(w http.ResponseWriter, _ *http.Request) {
	ports, err := serialports.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ports": ports})
}

func (s *Server) openRTUMaster(w http.ResponseWriter, r *http.Request) {
	var cfg modbus.SerialConfig
	if err := decodeJSON(w, r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, slaveOpen, _ := s.cfg.RTUSlave.Status(); slaveOpen {
		writeError(w, http.StatusConflict, "close RTU slave before opening RTU master")
		return
	}
	if err := s.cfg.RTUMaster.Open(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": cfg})
}

func (s *Server) closeRTUMaster(w http.ResponseWriter, _ *http.Request) {
	if err := s.cfg.RTUMaster.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) rtuRequest(w http.ResponseWriter, r *http.Request) {
	var req modbus.Request
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.cfg.RTUMaster.Execute(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) startRTUSlave(w http.ResponseWriter, r *http.Request) {
	var cfg modbus.SerialConfig
	if err := decodeJSON(w, r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, masterOpen := s.cfg.RTUMaster.Config(); masterOpen {
		writeError(w, http.StatusConflict, "close RTU master before starting RTU slave")
		return
	}
	if err := s.cfg.RTUSlave.Start(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": cfg})
}

func (s *Server) stopRTUSlave(w http.ResponseWriter, _ *http.Request) {
	if err := s.cfg.RTUSlave.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) rtuStatus(w http.ResponseWriter, _ *http.Request) {
	masterCfg, masterOpen := s.cfg.RTUMaster.Config()
	slaveCfg, slaveOpen, stats := s.cfg.RTUSlave.Status()
	writeJSON(w, http.StatusOK, map[string]any{"master": map[string]any{"open": masterOpen, "config": masterCfg}, "slave": map[string]any{"open": slaveOpen, "config": slaveCfg, "stats": stats}})
}

func (s *Server) modbusRequest(w http.ResponseWriter, r *http.Request) {
	var req modbus.Request
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := modbus.ExecuteTCP(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) simulatorProfile(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, modbus.SWSLabProfile())
}

func (s *Server) resetSimulator(w http.ResponseWriter, _ *http.Request) {
	if err := modbus.ResetSWSLab(s.cfg.Store); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "profile": modbus.SWSLabProfile().ID})
}

type simulatorWrite struct {
	SlaveID uint8    `json:"slaveId"`
	Table   string   `json:"table"`
	Address uint16   `json:"address"`
	Values  []uint16 `json:"values"`
}

func (s *Server) writeSimulatorRegisters(w http.ResponseWriter, r *http.Request) {
	var body simulatorWrite
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.SlaveID < 1 || body.SlaveID > 247 {
		writeError(w, http.StatusBadRequest, "slaveId must be 1..247")
		return
	}
	table, err := parseTable(body.Table)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Values) < 1 || len(body.Values) > 2000 {
		writeError(w, http.StatusBadRequest, "values must contain 1..2000 entries")
		return
	}
	if uint32(body.Address)+uint32(len(body.Values)) > 65536 {
		writeError(w, http.StatusBadRequest, "register range exceeds 65535")
		return
	}
	if table == modbus.Holding || table == modbus.Coil {
		err = s.cfg.Store.Write(table, body.SlaveID, body.Address, body.Values)
	} else {
		err = s.cfg.Store.Set(table, body.SlaveID, body.Address, body.Values)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	values, err := s.cfg.Store.Read(table, body.SlaveID, body.Address, uint16(len(body.Values)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slaveId": body.SlaveID, "table": table, "address": body.Address, "values": values})
}

func parseTable(value string) (modbus.Table, error) {
	table := modbus.Table(value)
	switch table {
	case modbus.Coil, modbus.Discrete, modbus.Holding, modbus.Input:
		return table, nil
	default:
		return "", errors.New("table must be coil, discrete, holding or input")
	}
}

func parseSlave(r *http.Request) (uint8, error) {
	value, err := strconv.Atoi(r.PathValue("slaveId"))
	if err != nil || value < 1 || value > 247 {
		return 0, errors.New("slaveId must be 1..247")
	}
	return uint8(value), nil
}

func (s *Server) readRegisters(w http.ResponseWriter, r *http.Request) {
	slave, err := parseSlave(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	table, err := parseTable(r.URL.Query().Get("table"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	address, err := parseUint16(r.URL.Query().Get("address"), "address")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	quantity64, err := strconv.ParseUint(r.URL.Query().Get("quantity"), 10, 16)
	maxQuantity := uint64(125)
	if table == modbus.Coil || table == modbus.Discrete {
		maxQuantity = 2000
	}
	if err != nil || quantity64 < 1 || quantity64 > maxQuantity {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("quantity must be 1..%d", maxQuantity))
		return
	}
	if uint64(address)+quantity64 > 65536 {
		writeError(w, http.StatusBadRequest, "register range exceeds 65535")
		return
	}
	values, err := s.cfg.Store.Read(table, slave, address, uint16(quantity64))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"slaveId": slave, "table": table, "address": address, "quantity": len(values), "values": values})
}

type registerWrite struct {
	Table   string   `json:"table"`
	Address uint16   `json:"address"`
	Values  []uint16 `json:"values"`
}

func (s *Server) writeRegisters(w http.ResponseWriter, r *http.Request) {
	slave, err := parseSlave(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body registerWrite
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	table, err := parseTable(body.Table)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if table != modbus.Holding && table != modbus.Coil {
		writeError(w, http.StatusBadRequest, "only holding and coil registers are writable")
		return
	}
	maxValues := 123
	if table == modbus.Coil {
		maxValues = 1968
	}
	if len(body.Values) < 1 || len(body.Values) > maxValues {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("values must contain 1..%d entries", maxValues))
		return
	}
	if uint32(body.Address)+uint32(len(body.Values)) > 65536 {
		writeError(w, http.StatusBadRequest, "register range exceeds 65535")
		return
	}
	if err := s.cfg.Store.Write(table, slave, body.Address, body.Values); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	values, _ := s.cfg.Store.Read(table, slave, body.Address, uint16(len(body.Values)))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slaveId": slave, "table": table, "address": body.Address, "values": values})
}

func parseUint16(value, name string) (uint16, error) {
	parsed, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("%s must be 0..65535", name)
	}
	return uint16(parsed), nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	defer func() { _ = r.Body.Close() }()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("invalid JSON: multiple values are not allowed")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message, "status": status, "at": time.Now().UTC().Format(time.RFC3339)})
}
