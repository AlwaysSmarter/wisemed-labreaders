package comm

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"wisemed-labreaders/comm/serial"
	"wisemed-labreaders/comm/tcpip"
	"wisemed-labreaders/config"
)

func IsEquipmentCommunicationActive() bool {
	commOK, err := AreLISCommunicationParamsOK(config.ServerConfiguration)
	if err != nil || !commOK {
		log.Printf("Communication cannot be started as it misses proper communication. %s", err)
		return false
	}

	switch strings.ToLower(config.ServerConfiguration.CommType) {
	case "serial":
		return serial.IsCommunicationActive()
		break
	case "tcpip":
		return tcpip.IsCommunicationActive()
		break
	}
	return false
}

func ReStartEquipmentCommunication(createProtoHandler config.CreateProtocolHandler) error {
	commOK, err := AreLISCommunicationParamsOK(config.ServerConfiguration)
	if err != nil || !commOK {
		log.Printf("Communication cannot be started as it misses proper communication. %s", err)
		return err
	}

	log.Println("(re)Starting Communication")
	EndEquipmentCommunication()
	switch strings.ToLower(config.ServerConfiguration.CommType) {
	case "serial":
		err = serial.StartSerialCommunication(createProtoHandler)
		break
	case "tcpip":
		err = tcpip.StartTCPIPCommunication(createProtoHandler)
		break
	default:
		return errors.New("Unknown communication type: " + config.ServerConfiguration.CommType)
		break
	}
	if err != nil {
		log.Println("Could not (re)start communication")
		return err
	}
	log.Println("Communication (re)Started")
	return nil
}

func EndEquipmentCommunication() error {
	log.Println("Stopping Communication")
	var err error
	switch strings.ToLower(config.ServerConfiguration.CommType) {
	case "serial":
		err = serial.EndSerialCommunication()
		break
	case "tcpip":
		tcpip.EndTCPIPCommunication()
		break
	}

	if err != nil {
		log.Println("Could not stop communication", err)
		return err
	}
	log.Print("Communication Stopped")
	return nil
}

func IetLISCommunicationParam(param string) bool {
	commParams := getLISCommunicationParams()
	if _, ok := commParams[param]; ok {
		return true
	}
	return false
}

func getLISCommunicationParams() map[string]string {
	return map[string]string{
		"cfg_comm_type":             "cfg_comm_type",
		"cfg_comm_serial_port":      "cfg_comm_serial_port",
		"cfg_comm_serial_baud":      "cfg_comm_serial_baud",
		"cfg_comm_serial_parity":    "cfg_comm_serial_parity",
		"cfg_comm_serial_stop_bits": "cfg_comm_serial_stop_bits",
		"cfg_comm_tcpip_type":       "cfg_comm_tcpip_type",
		"cfg_comm_tcpip_addr":       "cfg_comm_tcpip_addr",
		"cfg_comm_tcpip_port":       "cfg_comm_tcpip_port",
	}
}

func AreLISCommunicationParamsOK(srvConfig config.WMLRAPIConfigServer) (bool, error) {
	switch srvConfig.CommType {
	case "TCPIP":
		tmpInt, err := strconv.Atoi(srvConfig.CommTCPIPType)
		if err != nil {
			return false, errors.New("Unknown TCPIP communication type")
		}
		if tmpInt <= 0 || tmpInt > 2 {
			return false, errors.New(fmt.Sprintf("Unknown TCPIP communication type (%d)", tmpInt))
		}
		if net.ParseIP(srvConfig.CommTCPIPAddress) == nil {
			return false, errors.New(fmt.Sprintf("Invalid TCPIP address %s", srvConfig.CommTCPIPAddress))
		}
		tmpInt, err = strconv.Atoi(srvConfig.CommTCPIPPort)
		if err != nil {
			return false, errors.New("Unknown TCPIP port")
		}
		if tmpInt <= 0 || tmpInt >= 65500 {
			return false, errors.New(fmt.Sprintf("Invalid TCPIP port %d", tmpInt))
		}
		return true, nil
		break
	case "SERIAL":
		if srvConfig.CommSerialPort == "" {
			return false, errors.New("Serial port is void")
		}
		tmpInt, err := strconv.Atoi(srvConfig.CommSerialStopBits)
		if err != nil {
			return false, errors.New("Unknown serial stop bits")
		}
		if tmpInt <= 0 || tmpInt > 2 {
			return false, errors.New(fmt.Sprintf("Unknown serial stop bits (%d)", tmpInt))
		}
		tmpInt, err = strconv.Atoi(srvConfig.CommSerialParity)
		if err != nil {
			return false, errors.New("Unknown serial parity")
		}
		if tmpInt <= 0 || tmpInt > 3 {
			return false, errors.New(fmt.Sprintf("Unknown serial parity (%d)", tmpInt))
		}
		tmpInt, err = strconv.Atoi(srvConfig.CommSerialBaud)
		if err != nil {
			return false, errors.New("Unknown serial baud rate")
		}
		if tmpInt != 110 && tmpInt != 300 && tmpInt != 600 && tmpInt != 1200 && tmpInt != 2400 && tmpInt != 4800 && tmpInt != 9600 && tmpInt != 14400 && tmpInt != 19200 && tmpInt != 38400 && tmpInt != 57600 && tmpInt != 115200 && tmpInt != 128000 && tmpInt != 256000 {
			return false, errors.New(fmt.Sprintf("Unknown serial parity (%d)", tmpInt))
		}
		return true, nil
		break
	}
	return false, errors.New("Unknown communication type")
}

func BroadcastMessage(createProtoHandler config.CreateProtocolHandler, msg string) error {
	switch strings.ToLower(config.ServerConfiguration.CommType) {
	case "tcpip":
		return tcpip.BroadcastMessage(createProtoHandler, msg)
	case "serial":
		return serial.BroadcastMessage(createProtoHandler, msg)
	}
	return nil
}

func TestCommunication(createProtoHandler config.CreateProtocolHandler, msg string, args ...interface{}) error {
	switch strings.ToLower(config.ServerConfiguration.CommType) {
	case "tcpip":
		return tcpip.TestCommunication(createProtoHandler, msg, args...)
	case "serial":
		return serial.TestCommunication(createProtoHandler, msg, args...)
	}
	return errors.New("Unknown communication channel")
}
func InitiateCommand(createProtoHandler config.CreateProtocolHandler, msg string, args ...interface{}) (int, error) {
	switch strings.ToLower(config.ServerConfiguration.CommType) {
	case "tcpip":
		return tcpip.InitiateCommand(createProtoHandler, msg, args...)
	case "serial":
		return serial.InitiateCommand(createProtoHandler, msg, args...)
	}
	return 0, errors.New("Unknown communication channel")
}
