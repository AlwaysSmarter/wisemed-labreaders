package stxetxpackages

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"wisemed-labreaders/config"
	"wisemed-labreaders/general"
	"wisemed-labreaders/protocols"
	"wisemed-labreaders/sqlitewrapper"
	"wisemed-labreaders/wisemed"
)

type STXETXProtocol struct {
	STXETXId    string
	CommStarted bool
	STXSequence string
	ETXSequence string

	Data                  string
	AnalyzerConn          config.AnalyzerConnection
	STXETXBlocks          general.StringQueue
	SendOrdSegStringQueue general.StringQueue
	OrderReqestsQueue     general.ObjectQueue
	CROrderReqestsQueue   general.ObjectQueue
	ResultsFilesQueue     general.ObjectQueue

	EventDataSent                func(data string)
	EventBroadcastWsMessage      func(msgType string, data string)
	EventDataArrived             func(data string)
	EventSetInitialized          func(initialized bool)
	EventNewPatientResultArrived func(fileData interface{})

	FileData  sqlitewrapper.SQLOrder
	OrderData sqlitewrapper.SQLOrder

	CustomOnPackageReceiveReponder func(proto *STXETXProtocol, ac config.AnalyzerConnection, data string)
	CustomBlockDecoder             func(proto *STXETXProtocol, ac config.AnalyzerConnection, data string)
	CustomBlockLinesSplitter       func() string

	CustomReceivePackage func(proto *STXETXProtocol, data string) error

	CustomBuildQueryResponsePackage func(proto *STXETXProtocol, data string) string
}

var debugSTXETCLevel = 2 //2 - send as broadcast message
var MsgBUffer = ""
var communicationRestartedChannel = make(chan bool)
var communicationDataArrivedChannel = make(chan bool)
var communicationCheckPackagesToSendChannel = make(chan bool)

/** BEGIN ProtocolHandler interface **/
func (proto *STXETXProtocol) LogSTXETXMessage(data string, sendWMBroadcSTXETXessage bool, msgType string) {
	fmt.Printf(data)
	if sendWMBroadcSTXETXessage {
		proto.EventBroadcastWsMessage(msgType, data)
	}
}

func (proto *STXETXProtocol) SendPackagesIfAny(ac config.AnalyzerConnection) {
	proto.LogSTXETXMessage(fmt.Sprintf("\nSendPackagesIfAny for %q - Queue len; %d\n", proto.STXETXId, proto.SendOrdSegStringQueue.Len()), (debugSTXETCLevel > 1), "logamsg")
	if proto.SendOrdSegStringQueue.Len() > 0 {
		proto.SendString(ac, string(ENQ))
	} else {
		//restart timer for live check
		go func() { communicationRestartedChannel <- true }()
	}
}
func (proto *STXETXProtocol) SendString(ac config.AnalyzerConnection, data string) {
	if proto.EventDataSent != nil {
		proto.EventDataSent(data)
	}
	if ac != nil {
		ac.SendString(data)
	}
}
func (proto *STXETXProtocol) SendData(ac config.AnalyzerConnection, data []byte) {
	if proto.EventDataSent != nil {
		proto.EventDataSent(string(data))
	}
	if ac != nil {
		ac.SendData(data)
	}
}
func (proto *STXETXProtocol) OnDataArrived(ac config.AnalyzerConnection, data string) {
	if proto.EventDataArrived != nil {
		proto.EventDataArrived(data)
	}
}
func (proto *STXETXProtocol) OnSetInitialized(ac config.AnalyzerConnection, initialized bool) {
	if proto.EventSetInitialized != nil {
		proto.EventSetInitialized(initialized)
	}
}
func (proto *STXETXProtocol) OnNewPatientResultArrived(ac config.AnalyzerConnection, newFile interface{}) {
	if proto.EventNewPatientResultArrived != nil {
		proto.EventNewPatientResultArrived(newFile)
	}
}
func (proto *STXETXProtocol) InitiateCommand(ac config.AnalyzerConnection, cmd string, arg ...interface{}) {
	//
}
func (proto *STXETXProtocol) StartCommunication(ac config.AnalyzerConnection) {
	// Same as above, though since id may have already been destroyed
	// once, I name the channel different
	go func(t time.Duration, ac config.AnalyzerConnection) {
		proto.LogSTXETXMessage("StartCommunication ticker", (debugSTXETCLevel > 1), "logamsg")
		// Sends to the channel every t
		timeTicker := time.NewTicker(t)
		tick := timeTicker.C

		// Wrap, otherwise select will only execute the first tick
		for {
			select {
			// t has passed, so id can be destroyed
			case <-tick:
				timeTicker.Stop()
				fmt.Println("Ticker hit for %s", proto.STXETXId)
				proto.SendPackagesIfAny(ac)
				timeTicker.Reset(t)
				//return
				// We are finished destroying stuff
			case <-communicationRestartedChannel:
				timeTicker.Stop()
				proto.LogSTXETXMessage("StartCommunication ticker : communicationRestartedChannel", (debugSTXETCLevel > 1), "logamsg")
				timeTicker.Reset(t)
				//return
			case <-communicationDataArrivedChannel:
				timeTicker.Stop()
				proto.LogSTXETXMessage("StartCommunication ticker : communicationDataArrivedChannel", (debugSTXETCLevel > 1), "logamsg")
				//return
			case <-communicationCheckPackagesToSendChannel:
				timeTicker.Stop()
				proto.LogSTXETXMessage("StartCommunication ticker : communicationCheckPackagesToSendChannel", (debugSTXETCLevel > 1), "logamsg")
				timeTicker.Reset(1 * time.Second)
				//return
			}
		}
	}(time.Second*60, ac)
}
func (proto *STXETXProtocol) TestCommunication(ac config.AnalyzerConnection, commData string) {
	proto.LogSTXETXMessage(fmt.Sprintf("TESENVT:\nWill start communication for %q", &proto), (debugSTXETCLevel > 1), "logamsg")
	if !proto.CommStarted {
		proto.StartCommunication(ac)
		proto.CommStarted = true
	}
	dataArr := strings.Split(commData, "\r\n")

	for _, data := range dataArr {
		data = strings.ReplaceAll(data, "\\x02", string(rune(2)))
		data = strings.ReplaceAll(data, "\\x03", string(rune(3)))
		data = strings.ReplaceAll(data, "\\x04", string(rune(4)))
		data = strings.ReplaceAll(data, "\\x05", string(rune(5)))
		data = strings.ReplaceAll(data, "\\x06", string(rune(6)))
		data = strings.ReplaceAll(data, "\\x32", " ")
		data = strings.ReplaceAll(data, "\\r", string(rune(CR)))
		data = strings.ReplaceAll(data, "\\n", string(rune(LF)))

		data = strings.ReplaceAll(data, "\\\"", string(rune(34)))
		data = strings.ReplaceAll(data, "\\\\", "\\")
		proto.ParseCluster(ac, data)
	}
}
func (proto *STXETXProtocol) HasValidPatient() bool {
	return (proto.FileData.PatientId != "0" && proto.FileData.PatientId != "") || (proto.FileData.FileId != "0" && proto.FileData.FileId != "")
}
func (proto *STXETXProtocol) HasValidOrder() bool {
	return (proto.OrderData.PatientId != "0" && proto.OrderData.PatientId != "") || (proto.OrderData.FileId != "0" && proto.OrderData.FileId != "")
}
func (proto *STXETXProtocol) ParseCluster(ac config.AnalyzerConnection, data string) {
	go func() { communicationDataArrivedChannel <- true }()
	if protocols.StrLen(data) <= 0 {
		return
	}
	MsgBUffer = fmt.Sprintf("%s%s", MsgBUffer, data)

	if proto.OnDataArrived != nil {
		proto.OnDataArrived(ac, data)
	}

	if proto.CustomOnPackageReceiveReponder != nil {
		proto.CustomOnPackageReceiveReponder(proto, ac, data)
	}

	if proto.STXSequence == "" {
		proto.STXSequence = string(STX)
	}
	if proto.ETXSequence == "" {
		proto.ETXSequence = string(ETX)
	}
	MsgBUffer = proto.STXETXBlocks.SplitBlocksUTF8(MsgBUffer, proto.STXSequence, proto.ETXSequence, false)

	go proto.parseSTXETXBlocks(ac)
}

func (proto *STXETXProtocol) parseSTXETXBlocks(ac config.AnalyzerConnection) {

	for {
		if proto.STXETXBlocks.Len() <= 0 {
			break
		}
		block, err := proto.STXETXBlocks.Pop()
		if err != nil {
			proto.LogSTXETXMessage(fmt.Sprintf("An error has occured!\n%v", err), true, "logaerr")
			return
		}
		proto.LogSTXETXMessage(fmt.Sprintf("I have a new STXETX block:\n\n%q\n\n", block), true, "logamsg")

		if proto.CustomBlockDecoder != nil {
			proto.CustomBlockDecoder(proto, ac, block)
		} else {
			panic("Unknown block decoder")
		}

	}
}

/** END ProtocolHandler interface **/

func (proto *STXETXProtocol) SaveResultToDatabase() error {
	for proto.ResultsFilesQueue.Len() > 0 {
		tmpObj, err := proto.ResultsFilesQueue.Pop()
		if err != nil {
			proto.LogSTXETXMessage(fmt.Sprintf("Erroron save result: %v\n", err), true, "logaerr")
			return err
		}
		tmpFile, ok := tmpObj.(sqlitewrapper.SQLOrder)
		if !ok {
			proto.LogSTXETXMessage("Patient conversion error", true, "logaerr")
			return errors.New("Patient conversion error")
		}

		nowt := time.Now()
		//first load order from DB

		fileId, err := strconv.Atoi(tmpFile.FileId)
		if err != nil || fileId <= 0 {
			proto.LogSTXETXMessage("FileID is not a positive integer - aborting the save", true, "logaerr")
			continue
		}

		readerOrderSQLITE, _, err := wisemed.LoadFileFromWMAsObj(nowt.Format("2006-01-02"), -1, -1, -1, fileId)
		if err != nil {
			return err
		}
		readerOrderSQLITE.FormatDatesForDB()
		proto.LogSTXETXMessage(fmt.Sprintf("\nFile loaded from WM:\n%d name: %s", readerOrderSQLITE.FileId, readerOrderSQLITE.PatientName), true, "logamsg")
		proto.LogSTXETXMessage(fmt.Sprintf("\nTests on file (save):"), true, "logamsg")

		for tstIdx, tst := range readerOrderSQLITE.Tests {
			proto.LogSTXETXMessage(fmt.Sprintf("\n%d - %s [%s] (%s);", tstIdx, tst.Name, tst.Code, tst.Tag), true, "logamsg")
		}
		someDataChanged := false

		for _, test := range tmpFile.Tests {
			proto.LogSTXETXMessage(fmt.Sprintf("Searching for test: %s, ", test.Code), true, "logamsg")
			foundTest := ""
			for tstIdx, tst := range readerOrderSQLITE.Tests {
				if test.Code == tst.Code {
					readerOrderSQLITE.Tests[tstIdx].Raw = test.Raw
					someDataChanged = true
					foundTest = test.Raw
					break
				}
			}
			if foundTest == "" {
				test.Name = "[an]" + test.Code
				test.OrderId = readerOrderSQLITE.Id
				someDataChanged = true
				readerOrderSQLITE.Tests = append(readerOrderSQLITE.Tests, test)
				proto.LogSTXETXMessage("Not found - adding as analyzer test", true, "logamsg")
			} else {
				proto.LogSTXETXMessage("Found", true, "logamsg")
			}
		}
		if someDataChanged {
			err = readerOrderSQLITE.SaveFromCommToDB()
			if err != nil {
				proto.LogSTXETXMessage(fmt.Sprintf("I have an error on saving to database:\n %q", err), true, "logaerr")
				return err
			}
		}

	}

	return nil
}
