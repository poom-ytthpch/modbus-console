package modbus

// SimulatorProfile describes the built-in virtual Modbus bus used to test an
// SWS controller without physical downstream sensors or relay hardware.
type SimulatorProfile struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Devices     []SimulatedDevice `json:"devices"`
}

type SimulatedDevice struct {
	Key         string           `json:"key"`
	Name        string           `json:"name"`
	SlaveID     uint8            `json:"slaveId"`
	Description string           `json:"description"`
	Ranges      []SimulatedRange `json:"ranges"`
}

type SimulatedRange struct {
	Label        string       `json:"label"`
	Table        Table        `json:"table"`
	FunctionCode FunctionCode `json:"functionCode"`
	Address      uint16       `json:"address"`
	Values       []uint16     `json:"values"`
	Writable     bool         `json:"writable"`
	Note         string       `json:"note,omitempty"`
}

func SWSLabProfile() SimulatorProfile {
	offCoils := []uint16{1, 1, 1, 1, 1, 1, 1, 1}
	offHolding := []uint16{512, 512, 512, 512, 512, 512, 512, 512}
	return SimulatorProfile{
		ID:          "sws-lab-v1",
		Name:        "SWS Lab Device Bus",
		Description: "Virtual downstream Modbus devices for exercising SWS sensor polling and relay control without physical RS485 hardware.",
		Devices: []SimulatedDevice{
			{Key: "xy-md02", Name: "XY-MD02 Temperature / Humidity", SlaveID: 8, Description: "Factory temperature and humidity sensor preset.", Ranges: []SimulatedRange{{Label: "Temperature / Humidity", Table: Input, FunctionCode: 4, Address: 1, Values: []uint16{285, 720}, Writable: false, Note: "Preset corresponds to 28.5 C and 72.0 %RH with x0.1 scaling."}}},
			{Key: "flowmeter-15", Name: "Flowmeter ID 15", SlaveID: 15, Description: "Dynamic two-channel flowmeter used for UAT sensor provisioning tests.", Ranges: []SimulatedRange{{Label: "Flow rate / Total flow", Table: Holding, FunctionCode: 3, Address: 0, Values: []uint16{125, 4582}, Writable: true, Note: "HREG0=125 (12.5 L/min with x0.1 scaling), HREG1=4582 L total flow."}}},
			{Key: "ec4400", Name: "EC4400", SlaveID: 2, Description: "Factory EC sensor raw preset.", Ranges: []SimulatedRange{{Label: "EC raw", Table: Holding, FunctionCode: 3, Address: 0, Values: []uint16{1800}, Writable: true}}},
			{Key: "sensor-6", Name: "Factory Sensor ID 6", SlaveID: 6, Description: "Two-channel factory sensor preset.", Ranges: []SimulatedRange{{Label: "Channels", Table: Holding, FunctionCode: 3, Address: 0, Values: []uint16{65, 55}, Writable: true}}},
			{Key: "sensor-1", Name: "Factory Sensor ID 1", SlaveID: 1, Description: "Two-channel factory sensor preset.", Ranges: []SimulatedRange{{Label: "Channels", Table: Holding, FunctionCode: 3, Address: 0, Values: []uint16{270, 72}, Writable: true}}},
			{Key: "meter-3", Name: "Factory Meter ID 3", SlaveID: 3, Description: "Two-register meter preset.", Ranges: []SimulatedRange{{Label: "Meter words", Table: Holding, FunctionCode: 3, Address: 0, Values: []uint16{1234, 56700}, Writable: true}}},
			{Key: "chlorine", Name: "Chlorine Sensor", SlaveID: 4, Description: "Chlorine float32 preset.", Ranges: []SimulatedRange{{Label: "Chlorine float32 ABCD", Table: Holding, FunctionCode: 3, Address: 2, Values: []uint16{0x4020, 0x0000}, Writable: true, Note: "0x40200000 is IEEE-754 float32 value 2.5."}}},
			{Key: "relay-8ch", Name: "8-Channel Modbus Relay", SlaveID: 5, Description: "SWS active-low relay model. Coil raw 0 = ON, raw 1 = OFF. Holding 256 = ON, 512 = OFF.", Ranges: []SimulatedRange{
				{Label: "Relay coils", Table: Coil, FunctionCode: 1, Address: 0, Values: offCoils, Writable: true, Note: "Use FC05/FC15. Active-low: 0=ON, 1=OFF."},
				{Label: "Relay holding mirror", Table: Holding, FunctionCode: 3, Address: 1, Values: offHolding, Writable: true, Note: "Use FC06/FC16. 256=ON, 512=OFF. Coil and holding views stay synchronized."},
			}},
		},
	}
}

// ResetSWSLab restores every built-in simulator value to its deterministic
// initial state. It intentionally uses Set for sensor banks and EnableRelay for
// the relay so tests can mutate even read-only Modbus tables through the admin
// simulator control plane and then restore them.
func ResetSWSLab(store *Store) error {
	profile := SWSLabProfile()
	for _, device := range profile.Devices {
		if device.Key == "relay-8ch" {
			continue
		}
		for _, registerRange := range device.Ranges {
			if err := store.Set(registerRange.Table, device.SlaveID, registerRange.Address, registerRange.Values); err != nil {
				return err
			}
		}
	}
	return store.EnableRelay(RelaySpec{SlaveID: 5, ChannelCount: 8, CoilStart: 0, HoldingStart: 1, CoilOn: 0, CoilOff: 1, HoldingOn: 256, HoldingOff: 512})
}
