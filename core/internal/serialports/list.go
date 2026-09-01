package serialports

import "go.bug.st/serial"

type Port struct {
	Name string `json:"name"`
}

func List() ([]Port, error) {
	names, err := serial.GetPortsList()
	if err != nil {
		return nil, err
	}
	ports := make([]Port, 0, len(names))
	for _, name := range names {
		ports = append(ports, Port{Name: name})
	}
	return ports, nil
}
