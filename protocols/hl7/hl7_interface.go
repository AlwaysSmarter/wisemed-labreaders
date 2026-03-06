package hl7

import (
	"bytes"
	"fmt"
	"github.com/lenaten/hl7"
	"strings"
	"time"
	"wisemed-labreaders/config"
	"wisemed-labreaders/general"
	"wisemed-labreaders/protocols"
	"wisemed-labreaders/protocols/hl7/hl7_handlers"
	"wisemed-labreaders/protocols/hl7/hl7_segments"
	"wisemed-labreaders/sqlitewrapper"
)

var msgBUffer = ""
var communicationRestartedChannel = make(chan bool)
var communicationDataArrivedChannel = make(chan bool)
var communicationCheckPackagesToSendChannel = make(chan bool)

type HL7Protocol struct {
	HL7Id                   string
	CommStarted             bool
	debugHL7Level           int
	CFGRestrictToHL7Ver     string
	CFGBlockUnknownSerial   string
	CFGBlockUnknownPassword string

	Data         string
	AnalyzerConn config.AnalyzerConnection

	SendStringQueue       general.StringQueue
	ResultsFilesQueue     general.ObjectQueue
	SendOrdSegStringQueue general.StringQueue

	EventDataSent                func(data string)
	EventBroadcastWsMessage      func(msgType string, data string)
	EventDataArrived             func(data string)
	EventSetInitialized          func(initialized bool)
	EventNewPatientResultArrived func(fileData interface{})

	HandleHL7MsgType_EquipmentStatusMessage  func(msg *hl7.Message) ([]byte, error)
	HandleHL7MsgType_InventoryUpdateMessage  func(msg *hl7.Message) ([]byte, error)
	HandleHL7MsgType_TestSelectionInquiry    func(msg *hl7.Message, msgChan chan string, stopChan chan bool, errChan chan error)
	HandleHL7MsgType_TestSelectionInquiryACK func(msg *hl7.Message) error
	HandleHL7MsgType_MeasurementResults      func(msg *hl7.Message, mbc config.WSMessageBroadcaster, ac config.AnalyzerConnection) ([]string, error)

	FileData sqlitewrapper.SQLOrder

	CustomPatientDecoder func(proto *HL7Protocol, HL7Rec interface{})
	CustomOrderDecoder   func(proto *HL7Protocol, HL7Rec interface{})
	CustomResultDecoder  func(proto *HL7Protocol, HL7Rec interface{})
	CustomISQcDecoder    func(proto *HL7Protocol, HL7Rec interface{}) sqlitewrapper.SQLOrderResultType
}

func (proto *HL7Protocol) InitiateCommand(ac config.AnalyzerConnection, cmd string, arg ...interface{}) {
	switch cmd {
	case "reloadknowntests":
		cmd, err := hl7_handlers.GetHL7MsgType_InventoryRequest()
		if err != nil {
			fmt.Println("I have an error: ", err)
		} else {
			ac.SendString(cmd)
		}
		break
	}

}
func (proto *HL7Protocol) StartCommunication(ac config.AnalyzerConnection) {
	//// Same as above, though since id may have already been destroyed
	//// once, I name the channel different
	//go func(t time.Duration, ac config.AnalyzerConnection) {
	//
	//	// Sends to the channel every t
	//	tick := time.NewTicker(t).C
	//
	//	// Wrap, otherwise select will only execute the first tick
	//	for {
	//		select {
	//		// t has passed, so id can be destroyed
	//		case <-tick:
	//			//ac.SendString(string(ENQ))
	//		// We are finished destroying stuff
	//		case <-communicationStartedChannel:
	//			tick = time.NewTicker(t).C
	//			return
	//		}
	//	}
	//}(time.Second*60, ac)
	// Same as above, though since id may have already been destroyed
	// once, I name the channel different
	go func(t time.Duration, ac config.AnalyzerConnection) {
		return
		if proto.debugHL7Level > 10 {
			proto.LogHL7Message("StartCommunication ticker", (proto.debugHL7Level > 1), "logamsg")
		}
		// Sends to the channel every t
		timeTicker := time.NewTicker(t)
		tick := timeTicker.C

		// Wrap, otherwise select will only execute the first tick
		for {
			select {
			// t has passed, so id can be destroyed
			case <-tick:
				timeTicker.Stop()
				if proto.debugHL7Level > 10 {
					fmt.Println("Ticker hit for %s", proto.HL7Id)
				}
				proto.SendPackagesIfAny(ac)
				timeTicker.Reset(t)
				//return
				// We are finished destroying stuff
			case <-communicationRestartedChannel:
				timeTicker.Stop()
				if proto.debugHL7Level > 10 {
					proto.LogHL7Message("StartCommunication ticker : communicationRestartedChannel", (proto.debugHL7Level > 1), "logamsg")
				}
				timeTicker.Reset(t)
				//return
			case <-communicationDataArrivedChannel:
				timeTicker.Stop()
				if proto.debugHL7Level > 10 {
					proto.LogHL7Message("StartCommunication ticker : communicationDataArrivedChannel", (proto.debugHL7Level > 1), "logamsg")
				}
				//return
			case <-communicationCheckPackagesToSendChannel:
				timeTicker.Stop()
				if proto.debugHL7Level > 10 {
					proto.LogHL7Message("StartCommunication ticker : communicationCheckPackagesToSendChannel", (proto.debugHL7Level > 1), "logamsg")
				}
				timeTicker.Reset(1 * time.Second)
				//return
			}
		}
	}(time.Second*60, ac)
}

/** BEGIN ProtocolHandler interface **/
func (proto *HL7Protocol) LogHL7Message(data string, sendWMBroadcastMessage bool, msgType string) {
	fmt.Printf(data)
	if sendWMBroadcastMessage {
		proto.EventBroadcastWsMessage(msgType, data)
	}
}

func (proto *HL7Protocol) SendPackagesIfAny(ac config.AnalyzerConnection) {
	if proto.debugHL7Level > 10 {
		proto.LogHL7Message(fmt.Sprintf("\nSendPackagesIfAny for %q - Queue len; %d\n", proto.HL7Id, proto.SendOrdSegStringQueue.Len()), (proto.debugHL7Level > 1), "logamsg")
	}
	if proto.SendOrdSegStringQueue.Len() > 0 {
		proto.SendString(ac, string(ENQ))
	} else {
		//restart timer for live check
		go func() { communicationRestartedChannel <- true }()
	}
}

func (proto *HL7Protocol) SendString(ac config.AnalyzerConnection, data string) {
	proto.LogHL7Message(fmt.Sprintf("\nSendString for %q - %s\n", proto.HL7Id, data), (proto.debugHL7Level > 1), "logamsg")

	if proto.EventDataSent != nil {
		proto.EventDataSent(data)
	}
	if ac != nil {
		ac.SendString(data)
	} else {
		if proto.debugHL7Level > 10 {
			fmt.Printf("AC is nil but I would have send:\n<--- %q", data)
		}
	}
}
func (proto *HL7Protocol) SendData(ac config.AnalyzerConnection, data []byte) {
	proto.LogHL7Message(fmt.Sprintf("\nSendData for %q - %q\n", proto.HL7Id, data), (proto.debugHL7Level > 1), "logamsg")
	if proto.EventDataSent != nil {
		proto.EventDataSent(string(data))
	}
	if ac != nil {
		ac.SendData(data)
	} else {
		if proto.debugHL7Level > 10 {
			fmt.Printf("AC is nil but I would have send:\n<--- %q", data)
		}
	}
}
func (proto *HL7Protocol) BroadcastWSMessage(ac config.AnalyzerConnection, msgType string, data string) {
	if proto.EventBroadcastWsMessage != nil {
		proto.EventBroadcastWsMessage(msgType, data)
	}
}
func (proto *HL7Protocol) OnDataArrived(ac config.AnalyzerConnection, data string) {
	fmt.Printf("\n HL7 On Data Arrived ====[%s][%s]====> %s", ac.GetConnId(), proto.HL7Id, data)
	if proto.EventDataArrived != nil {
		proto.EventDataArrived(data)
	}
}
func (proto *HL7Protocol) OnSetInitialized(ac config.AnalyzerConnection, initialized bool) {
	if proto.EventSetInitialized != nil {
		proto.EventSetInitialized(initialized)
	}
}
func (proto *HL7Protocol) OnNewPatientResultArrived(ac config.AnalyzerConnection, newFile interface{}) {
	if proto.EventNewPatientResultArrived != nil {
		proto.EventNewPatientResultArrived(newFile)
	}
}
func (proto *HL7Protocol) HasValidPatient() bool {
	return proto.FileData.PatientId != "0" || proto.FileData.FileId != "0"
}
func (proto *HL7Protocol) TestCommunication(ac config.AnalyzerConnection, commData string) {
	if proto.debugHL7Level > 10 {
		proto.LogHL7Message(fmt.Sprintf("TESENVT:\nWill start communication for %q", &proto), (proto.debugHL7Level > 1), "logamsg")
	}
	if !proto.CommStarted {
		proto.StartCommunication(ac)
		proto.CommStarted = true
	}
	dataArr := strings.Split(commData, "\n")

	for _, data := range dataArr {
		data = strings.ReplaceAll(data, "\\v", string(rune(VT)))
		data = strings.ReplaceAll(data, "\\r", string(rune(CR)))
		data = strings.ReplaceAll(data, "\\n", string(rune(CR)))
		data = strings.ReplaceAll(data, "\\x1c", string(rune(FS)))
		data = strings.ReplaceAll(data, "\\\"", string(rune(34)))
		data = strings.ReplaceAll(data, "\\\\", "\\")
		proto.ParseCluster(ac, data)
	}

}
func (proto *HL7Protocol) ParseCluster(ac config.AnalyzerConnection, data string) {
	go func() { communicationDataArrivedChannel <- true }()

	if ac != nil {
		fmt.Printf("\n ParseCluster [%s][%s]:  %s", ac.GetConnId(), proto.HL7Id, data)
	}
	if protocols.StrLen(data) <= 0 {
		return
	}
	msgBUffer = fmt.Sprintf("%s%s", msgBUffer, data)

	hl7Block := protocols.GetPackage(&msgBUffer, string(VT), fmt.Sprintf("%s%s", string(FS), string(CR)), true, true)
	/*
		idx := strings.Index(msgBUffer, ) //start char
		if idx >= 0 {
			msgBUffer = msgBUffer[idx:]
		}

		idx = strings.Index(msgBUffer, string(FS)) //
		hl7Block := ""
		if idx >= 0 {
			hl7Block = fmt.Sprintf("%s%s", msgBUffer[:idx+1], string(CR))
			if idx+1 < len(msgBUffer) {
				msgBUffer = msgBUffer[idx+2:]
			}
		}
	*/

	if proto.OnDataArrived != nil && ac != nil {
		proto.OnDataArrived(ac, data)
	}
	if hl7Block == "" {
		return
	}
	if proto.debugHL7Level > 2 {
		fmt.Println("I have a new block")
	}

	// from an io.Reader
	reader := bytes.NewReader([]byte(hl7Block))
	msgs, err := NewDecoderUTF8(reader).Messages()
	if err != nil {
		return
	}
	for _, msg := range msgs {
		tmpSeg := hl7_segments.HL7MSH{}
		err := msg.Unmarshal(&tmpSeg)
		if err != nil {
			fmt.Println("An error has occured on unmarshaling the 1st segment")
			return
		}

		switch tmpSeg.MessageType {
		case hl7_handlers.HL7MessageTypesDefinitions.HL7MsgType_EquipmentStatusMessage:
			fmt.Println("I have a status message")
			if proto.HandleHL7MsgType_EquipmentStatusMessage == nil {
				proto.HandleHL7MsgType_EquipmentStatusMessage = hl7_handlers.HandleHL7MsgType_EquipmentStatusMessage
			}
			respondMessage, err := proto.HandleHL7MsgType_EquipmentStatusMessage(msg)
			if err != nil {
				fmt.Println("An error has occured on unmarshaling the 1st segment")
				return
			}
			proto.SendData(ac, respondMessage)
			break
		case hl7_handlers.HL7MessageTypesDefinitions.HL7MsgType_InventoryUpdateMessage:
			fmt.Println("I have an inventory update message")
			if proto.HandleHL7MsgType_InventoryUpdateMessage == nil {
				proto.HandleHL7MsgType_InventoryUpdateMessage = hl7_handlers.HandleHL7MsgType_InventoryUpdateMessage
			}
			respondMessage, err := proto.HandleHL7MsgType_InventoryUpdateMessage(msg)
			if err != nil {
				fmt.Println("An error has occured on unmarshaling the 1st segment")
				return
			}
			proto.SendData(ac, respondMessage)
			break
		case hl7_handlers.HL7MessageTypesDefinitions.HL7MsgType_TestSelectionInquiry:
			fmt.Println("I have an test selection inquiry ACK  message")
			msgChannel := make(chan string)
			errChannel := make(chan error)
			stopChannel := make(chan bool)
			if proto.HandleHL7MsgType_TestSelectionInquiry == nil {
				proto.HandleHL7MsgType_TestSelectionInquiry = hl7_handlers.HandleHL7MsgType_TestSelectionInquiry
			}
			go proto.HandleHL7MsgType_TestSelectionInquiry(msg, msgChannel, stopChannel, errChannel)
			if err != nil {
				fmt.Println("An error has occured on unmarshaling the 1st segment")
				return
			}

			timeTicker := time.NewTicker(time.Second * 18) //HL7 protocaol sets the minimum amount of time you have, to send your response back to an inquiry message is 18s
			tick := timeTicker.C

			fmt.Println("Waiting for messages")
			for {
				select {
				case msgToSend := <-msgChannel:
					fmt.Printf("\nNew message %q ", msgToSend)
					proto.SendString(ac, msgToSend)
				case <-stopChannel:
					fmt.Println("Stop message")
					return
				case errMsg := <-errChannel:
					fmt.Println("An error has occured on creating messages", errMsg)
					return
				case <-tick:
					fmt.Println("Timeout occured on waiting for messges to send - aborting by sending a termination package")
					//proto.SendString(ac, fmt.Sprintf("%s%s", string(FS), string(CR)))
					return
				}
			}

			break
		case hl7_handlers.HL7MessageTypesDefinitions.HL7MsgType_TestSelectionInquiryACK:
			fmt.Println("I have an test selection inquiry ACK  message")
			if proto.HandleHL7MsgType_TestSelectionInquiryACK == nil {
				proto.HandleHL7MsgType_TestSelectionInquiryACK = hl7_handlers.HandleHL7MsgType_TestSelectionInquiryACK
			}
			err := proto.HandleHL7MsgType_TestSelectionInquiryACK(msg)
			if err != nil {
				fmt.Println("An error has occured on unmarshaling the 1st segment")
				return
			}
			break
		case hl7_handlers.HL7MessageTypesDefinitions.HL7MsgType_MeasurementResults:
			fmt.Println("I have a measurement results message")
			if proto.HandleHL7MsgType_MeasurementResults == nil {
				proto.HandleHL7MsgType_MeasurementResults = hl7_handlers.HandleHL7MsgType_MeasurementResults
			}
			respondMessages, err := proto.HandleHL7MsgType_MeasurementResults(msg, proto, ac)
			if err != nil {
				fmt.Println("An error has occured on unmarshaling the 1st segment")
				return
			}
			for _, respondMessage := range respondMessages {
				proto.SendString(ac, respondMessage)
			}
			break
		}

	}
}

/** END ProtocolHandler interface **/
