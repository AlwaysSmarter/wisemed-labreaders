package analyzer

import (
	"fmt"

	"wisemed-labreaders/new/protocols/astm"
)

type Adapter interface {
	Code() string
	Name() string
	Category() string
	SupportsBidirectional() bool
	TransportOptions() []TransportOption
	DefaultTransport() TransportOption
	Status() map[string]interface{}
}

type TransportOption struct {
	Kind     string                 `json:"kind"`
	Mode     string                 `json:"mode"`
	Settings map[string]interface{} `json:"settings"`
}

type BasicAdapter struct {
	code          string
	name          string
	category      string
	protocol      string
	bidirectional bool
	transports    []TransportOption
	defaultIdx    int
}

func (a BasicAdapter) Code() string                { return a.code }
func (a BasicAdapter) Name() string                { return a.name }
func (a BasicAdapter) Category() string            { return a.category }
func (a BasicAdapter) SupportsBidirectional() bool { return a.bidirectional }
func (a BasicAdapter) TransportOptions() []TransportOption {
	return a.transports
}
func (a BasicAdapter) DefaultTransport() TransportOption {
	if a.defaultIdx >= 0 && a.defaultIdx < len(a.transports) {
		return a.transports[a.defaultIdx]
	}
	if len(a.transports) > 0 {
		return a.transports[0]
	}
	return TransportOption{}
}
func (a BasicAdapter) Status() map[string]interface{} {
	return map[string]interface{}{
		"adapter":       a.code,
		"name":          a.name,
		"category":      a.category,
		"protocol":      a.protocol,
		"bidirectional": a.bidirectional,
		"comm":          "ready",
	}
}

var adapters = map[string]Adapter{
	"cobas-pro": BasicAdapter{
		code: "cobas-pro", name: "Cobas PRO", category: "biochemistry", protocol: "HL7", bidirectional: true, defaultIdx: 0,
		transports: []TransportOption{
			{Kind: "network", Mode: "client", Settings: map[string]interface{}{"ip": "127.0.0.1", "port": 5000}},
			{Kind: "serial", Mode: "bidirectional", Settings: map[string]interface{}{"port": "/dev/tty.usbserial", "baud": 9600, "parity": "none", "stop_bits": 1}},
		},
	},
	"cobas-u411": BasicAdapter{
		code: "cobas-u411", name: "Cobas U411", category: "urine", protocol: "ASTM", bidirectional: true, defaultIdx: 0,
		transports: []TransportOption{
			{Kind: "serial", Mode: "bidirectional", Settings: map[string]interface{}{"port": "/dev/tty.usbserial", "baud": 9600, "parity": "none", "stop_bits": 1}},
			{Kind: "network", Mode: "server", Settings: map[string]interface{}{"ip": "0.0.0.0", "port": 5001}},
		},
	},
	"dirui-h800": BasicAdapter{
		code: "dirui-h800", name: "Dirui H800", category: "urine", protocol: "STX/ETX", bidirectional: true, defaultIdx: 0,
		transports: []TransportOption{
			{Kind: "network", Mode: "server", Settings: map[string]interface{}{"ip": "0.0.0.0", "port": 5002}},
		},
	},
	"indiko-plus": BasicAdapter{
		code: "indiko-plus", name: "Indiko Plus", category: "biochemistry", protocol: "ASTM", bidirectional: true, defaultIdx: 0,
		transports: []TransportOption{
			{Kind: "network", Mode: "client", Settings: map[string]interface{}{"ip": "127.0.0.1", "port": 5003}},
		},
	},
	"maglumi-800": BasicAdapter{
		code: "maglumi-800", name: "Maglumi 800", category: "immunology", protocol: astm.ProtocolName(), bidirectional: true, defaultIdx: 0,
		transports: []TransportOption{
			{Kind: "network", Mode: "server", Settings: map[string]interface{}{"ip": "0.0.0.0", "port": 5004}},
			{Kind: "file", Mode: "polling", Settings: map[string]interface{}{"directory": "./inbox", "mask": "*.txt", "poll_seconds": 2}},
		},
	},
	"mindray-bs600m": BasicAdapter{
		code: "mindray-bs600m", name: "Mindray BS600M", category: "biochemistry", protocol: "ASTM", bidirectional: true, defaultIdx: 0,
		transports: []TransportOption{
			{Kind: "serial", Mode: "bidirectional", Settings: map[string]interface{}{"port": "/dev/tty.usbserial", "baud": 9600, "parity": "none", "stop_bits": 1}},
		},
	},
	"sysmex-ca600": BasicAdapter{
		code: "sysmex-ca600", name: "Sysmex CA600", category: "coagulation", protocol: "ASTM", bidirectional: true, defaultIdx: 0,
		transports: []TransportOption{
			{Kind: "serial", Mode: "bidirectional", Settings: map[string]interface{}{"port": "/dev/tty.usbserial", "baud": 9600, "parity": "none", "stop_bits": 1}},
		},
	},
	"sysmex-xn550": BasicAdapter{
		code: "sysmex-xn550", name: "Sysmex XN550", category: "hematology", protocol: "HL7", bidirectional: true, defaultIdx: 0,
		transports: []TransportOption{
			{Kind: "network", Mode: "client", Settings: map[string]interface{}{"ip": "127.0.0.1", "port": 5005}},
		},
	},
	"cobas-pure": BasicAdapter{
		code: "cobas-pure", name: "Cobas Pure", category: "biochemistry", protocol: "HL7", bidirectional: true, defaultIdx: 0,
		transports: []TransportOption{
			{Kind: "network", Mode: "client", Settings: map[string]interface{}{"ip": "127.0.0.1", "port": 5006}},
			{Kind: "file", Mode: "polling", Settings: map[string]interface{}{"directory": "./inbox", "mask": "*.hl7", "poll_seconds": 2}},
		},
	},
}

func GetAdapter(code string) (Adapter, error) {
	a, ok := adapters[code]
	if !ok {
		return nil, fmt.Errorf("unknown analyzer adapter: %s", code)
	}
	return a, nil
}

func ListAdapters() []map[string]interface{} {
	res := make([]map[string]interface{}, 0, len(adapters))
	for _, a := range adapters {
		res = append(res, map[string]interface{}{
			"code":          a.Code(),
			"name":          a.Name(),
			"category":      a.Category(),
			"bidirectional": a.SupportsBidirectional(),
		})
	}
	return res
}
