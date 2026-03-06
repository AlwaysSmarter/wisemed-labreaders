package serial

import (
	"fmt"
	"go.bug.st/serial"
	"log"
	"sync"
	"wisemed-labreaders/config"
)

type SerialCommunicator struct {
	serialPort   string
	serialConfig *serial.Mode
	quit         chan interface{}
	wg           sync.WaitGroup
	ch           config.ProtocolHandler
	cph          config.CreateProtocolHandler
	done         chan bool
}

var serialComm *SerialCommunicator = nil

var cancelPortScanner func()

type SerialPortConnection struct {
	id          int
	connChannel serial.Port
}

func IsCommunicationActive() bool {
	if serialComm != nil {
		return true
	}
	return false
}
func (spc SerialPortConnection) GetConnId() string {
	return fmt.Sprintf("%d", spc.id)
}
func (spc SerialPortConnection) SendData(data []byte) {
	if spc.connChannel != nil {
		fmt.Printf("\n<---- %q\n", string(data))
		spc.connChannel.Write(data)
	}
}

func (spc SerialPortConnection) SendString(data string) {
	if spc.connChannel != nil {
		fmt.Printf("\n<---- %q\n", data)
		spc.connChannel.Write([]byte(data))
	}
}

func InitiateCommand(createProtoHandler config.CreateProtocolHandler, msg string, args ...interface{}) (int, error) {
	return 0, nil
}
func TestCommunication(createProtoHandler config.CreateProtocolHandler, cmd string, args ...interface{}) error {
	myCh := serialComm.ch
	if myCh == nil {
		myCh = serialComm.cph()
	}
	myCh.TestCommunication(nil, cmd)
	return nil
}
func BroadcastMessage(createProtoHandler config.CreateProtocolHandler, msg string) error {
	//BroadcastMessage
	return nil
}
func StartSerialCommunication(createProtoHandler config.CreateProtocolHandler) error {
	EndSerialCommunication()

	serialComm = &SerialCommunicator{
		quit:       make(chan interface{}),
		cph:        createProtoHandler,
		serialPort: config.ServerConfiguration.CommSerialPort,
		serialConfig: &serial.Mode{
			BaudRate: config.ReturnIntOrZero(config.ServerConfiguration.CommSerialBaud),
			//		DataBits:        byte(config.ReturnIntOrZero(config.ServerConfiguration.CommSerialStopBits)),
			//		StopBits:        byte(config.ReturnIntOrZero(config.ServerConfiguration.CommSerialStopBits)),
			//		Parity:      serial.Parity(config.ReturnIntOrZero(config.ServerConfiguration.CommSerialParity)),
		},
	}
	serialComm.wg.Add(1)
	go serialComm.serve()

	return nil
}

func EndSerialCommunication() error {
	if serialComm != nil {
		serialComm.quit <- 1
		<-serialComm.done
		serialComm = nil
	}
	return nil
}

func (s *SerialCommunicator) serve() {
	defer s.wg.Done()
	s.ch = s.cph()
	serialPortStream, err := serial.Open(s.serialPort, s.serialConfig)
	if err != nil {
		log.Println("ATTENTION!!!!! Cannot open COM port")
		return
	}

	var dataChan = make(chan []byte)
	s.ch.StartCommunication(&SerialPortConnection{id: 0, connChannel: serialPortStream})
	go func() {
		buff := make([]byte, 2048)
		for {
			n, err := serialPortStream.Read(buff)
			if err != nil {
				log.Println("ATTENTION!!!!! Cannot read from COM port")
				break
			}
			if n == 0 {
				fmt.Println("\nEOF")
				break
			}
			dataChan <- buff[:n]
		}
	}()

	for {
		select {
		case <-s.quit:
			s.done <- true
			return
		case newData := <-dataChan:
			s.ch.ParseCluster(&SerialPortConnection{id: 0, connChannel: serialPortStream}, string(newData))
		}
	}
}
