package astm

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

type ASTMProtocol struct {
	ASTMId                  string
	CommStarted             bool
	CFGRestrictToASTMVer    string
	CFGBlockUnknownSerial   string
	CFGBlockUnknownPassword string
	CFGPatientIDParser      []FieldParser
	CFGFileIDParser         []FieldParser

	ValidateChecksumAnyway bool
	Data                   string
	AnalyzerConn           config.AnalyzerConnection
	ASTMBlocks             general.StringQueue
	SendOrdSegStringQueue  general.StringQueue
	OrderReqestsQueue      general.ObjectQueue
	CROrderReqestsQueue    general.ObjectQueue
	ResultsFilesQueue      general.ObjectQueue

	EventDataSent                func(data string)
	EventBroadcastWsMessage      func(msgType string, data string)
	EventDataArrived             func(data string)
	EventSetInitialized          func(initialized bool)
	EventNewPatientResultArrived func(fileData interface{})

	FileData  sqlitewrapper.SQLOrder
	OrderData sqlitewrapper.SQLOrder

	CustomEachLineWithoutSTXETX     bool
	CustomEachLineWithoutSequenceNo bool
	CustomBlockDecoder              func(proto *ASTMProtocol, ac config.AnalyzerConnection, data string)
	CustomBlockLinesSplitter        func() string

	CustomReceiveSegment        func(proto *ASTMProtocol, ASTMRec ASTMSegmentInterface) error
	CustomReceiveHeaderSegment  func(proto *ASTMProtocol, ASTMRec *ASTMHeaderSegment) error
	CustomReceivePatientSegment func(proto *ASTMProtocol, ASTMRec *ASTMPIDSegment) error
	CustomReceiveQuerySegment   func(proto *ASTMProtocol, ASTMRec *ASTMRIRSegment) error
	CustomReceiveResultSegment  func(proto *ASTMProtocol, ASTMRec *ASTMResRecSegment) error
	CustomReceiveOrderSegment   func(proto *ASTMProtocol, ASTMRec *ASTMTORSegment) error
	CustomReceiveFinalSegment   func(proto *ASTMProtocol, ASTMRec *ASTMMTRSegment) error

	CustomBuildHeaderRecord            func(proto *ASTMProtocol, ASTMRec ASTMHeaderSegment) string
	CustomBuildPatientRecord           func(proto *ASTMProtocol, ASTMRec ASTMPIDSegment) string
	CustomBuildTestOrderRecord         func(proto *ASTMProtocol, ASTMRec []ASTMTORSegment) []string
	CustomBuildMessageTerminatorRecord func(proto *ASTMProtocol, ASTMRec ASTMMTRSegment) string
	//CustomBuildCommentRecord           func(proto *ASTMProtocol, ASTMRec ASTM) string
	//CustomBuildManufacturerInfoRecord  func(proto *ASTMProtocol, ASTMRec ASTMSegmentInterface) string
	//CustomBuildScientificInfoRecord    func(proto *ASTMProtocol, ASTMRec ASTMSegmentInterface) string

	CustomISQcDecoder func(proto *ASTMProtocol, ASTMRec *ASTMTORSegment) sqlitewrapper.SQLOrderResultType

	lastHeadSegment         ASTMHeaderSegment
	lastOrderInquirySegment ASTMRIRSegment
}

var debugASTMLevel = 2 //2 - send as broadcast message
var MsgBUffer = ""
var communicationRestartedChannel = make(chan bool)
var communicationDataArrivedChannel = make(chan bool)
var communicationCheckPackagesToSendChannel = make(chan bool)

var DefaultPatientIDParser = []FieldParser{
	FieldParser{GetFromField: "PracticePatID", GetFromFieldIdx: 2, ReturnType: "int"},
	FieldParser{GetFromField: "LabPatID", GetFromFieldIdx: 3, ReturnType: "int"},
	FieldParser{GetFromField: "PatID3", GetFromFieldIdx: 4, ReturnType: "int"},
}
var DefaultFileIDParser = []FieldParser{
	FieldParser{GetFromField: "InstrSpecimenID", GetFromFieldIdx: 3, ReturnType: "int", SplitFieldBy: "^", GetIdFromSplitIdx: 2},
	FieldParser{GetFromField: "SampleID", GetFromFieldIdx: 2, ReturnType: "int"},
}

/** BEGIN ProtocolHandler interface **/
func (proto *ASTMProtocol) LogASTMMessage(data string, sendWMBroadcastMessage bool, msgType string) {
	fmt.Printf(data)
	if sendWMBroadcastMessage {
		proto.EventBroadcastWsMessage(msgType, data)
	}
}

func (proto *ASTMProtocol) SendPackagesIfAny(ac config.AnalyzerConnection) {
	proto.LogASTMMessage(fmt.Sprintf("\nSendPackagesIfAny for %q - Queue len; %d\n", proto.ASTMId, proto.SendOrdSegStringQueue.Len()), (debugASTMLevel > 1), "logamsg")
	if proto.SendOrdSegStringQueue.Len() > 0 {
		proto.SendString(ac, string(ENQ))
	} else {
		//restart timer for live check
		go func() { communicationRestartedChannel <- true }()
	}
}
func (proto *ASTMProtocol) SendString(ac config.AnalyzerConnection, data string) {
	if proto.EventDataSent != nil {
		proto.EventDataSent(data)
	}
	if ac != nil {
		ac.SendString(data)
	}
}
func (proto *ASTMProtocol) SendData(ac config.AnalyzerConnection, data []byte) {
	if proto.EventDataSent != nil {
		proto.EventDataSent(string(data))
	}
	if ac != nil {
		ac.SendData(data)
	}
}
func (proto *ASTMProtocol) OnDataArrived(ac config.AnalyzerConnection, data string) {
	if proto.EventDataArrived != nil {
		proto.EventDataArrived(data)
	}
}
func (proto *ASTMProtocol) OnSetInitialized(ac config.AnalyzerConnection, initialized bool) {
	if proto.EventSetInitialized != nil {
		proto.EventSetInitialized(initialized)
	}
}
func (proto *ASTMProtocol) OnNewPatientResultArrived(ac config.AnalyzerConnection, newFile interface{}) {
	if proto.EventNewPatientResultArrived != nil {
		proto.EventNewPatientResultArrived(newFile)
	}
}
func (proto *ASTMProtocol) InitiateCommand(ac config.AnalyzerConnection, cmd string, arg ...interface{}) {
	//
}
func (proto *ASTMProtocol) StartCommunication(ac config.AnalyzerConnection) {
	// Same as above, though since id may have already been destroyed
	// once, I name the channel different
	go func(t time.Duration, ac config.AnalyzerConnection) {
		proto.LogASTMMessage("StartCommunication ticker", (debugASTMLevel > 1), "logamsg")
		// Sends to the channel every t
		timeTicker := time.NewTicker(t)
		tick := timeTicker.C

		// Wrap, otherwise select will only execute the first tick
		for {
			select {
			// t has passed, so id can be destroyed
			case <-tick:
				timeTicker.Stop()
				fmt.Println("Ticker hit for %s", proto.ASTMId)
				proto.SendPackagesIfAny(ac)
				timeTicker.Reset(t)
				//return
				// We are finished destroying stuff
			case <-communicationRestartedChannel:
				timeTicker.Stop()
				proto.LogASTMMessage("StartCommunication ticker : communicationRestartedChannel", (debugASTMLevel > 1), "logamsg")
				timeTicker.Reset(t)
				//return
			case <-communicationDataArrivedChannel:
				timeTicker.Stop()
				proto.LogASTMMessage("StartCommunication ticker : communicationDataArrivedChannel", (debugASTMLevel > 1), "logamsg")
				//return
			case <-communicationCheckPackagesToSendChannel:
				timeTicker.Stop()
				proto.LogASTMMessage("StartCommunication ticker : communicationCheckPackagesToSendChannel", (debugASTMLevel > 1), "logamsg")
				timeTicker.Reset(1 * time.Second)
				//return
			}
		}
	}(time.Second*60, ac)
}
func (proto *ASTMProtocol) TestCommunication(ac config.AnalyzerConnection, commData string) {
	proto.LogASTMMessage(fmt.Sprintf("TESENVT:\nWill start communication for %q", &proto), (debugASTMLevel > 1), "logamsg")
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
func (proto *ASTMProtocol) HasValidPatient() bool {
	return (proto.FileData.PatientId != "0" && proto.FileData.PatientId != "") || (proto.FileData.FileId != "0" && proto.FileData.FileId != "")
}
func (proto *ASTMProtocol) HasValidOrder() bool {
	return (proto.OrderData.PatientId != "0" && proto.OrderData.PatientId != "") || (proto.OrderData.FileId != "0" && proto.OrderData.FileId != "")
}
func (proto *ASTMProtocol) ParseCluster(ac config.AnalyzerConnection, data string) {
	go func() { communicationDataArrivedChannel <- true }()
	if protocols.StrLen(data) <= 0 {
		return
	}
	MsgBUffer = fmt.Sprintf("%s%s", MsgBUffer, data)
	firstChar := protocols.RuneAt(data, 0)

	if proto.OnDataArrived != nil {
		proto.OnDataArrived(ac, data)
	}

	firstChar = protocols.RuneAt(data, 0)

	switch firstChar {
	case ENQ:
		proto.LogASTMMessage("\nGot ENQ", (debugASTMLevel > 1), "logamsg")
		ASTM_prevSeqNo = 0
		proto.SendString(ac, string(ACK))
		if proto.OnSetInitialized != nil {
			proto.OnSetInitialized(ac, true)
		}
		break
	case ACK:
		proto.LogASTMMessage(fmt.Sprintf("\nGot ACK - i have %d elemens in sending queue\n", proto.SendOrdSegStringQueue.Len()), (debugASTMLevel > 1), "logamsg")
		el, err := proto.SendOrdSegStringQueue.Pop()
		if err != nil {
			proto.LogASTMMessage(fmt.Sprintf("\nError on popping the element %q - sending EOT\n", err), (debugASTMLevel > 1), "logamsg")
			proto.SendString(ac, string(EOT))
		} else {
			proto.LogASTMMessage(fmt.Sprintf("\nSending the element %s\n", el), (debugASTMLevel > 1), "logamsg")
			proto.SendString(ac, el)
		}
		break
	case NAK:
		proto.SendOrdSegStringQueue.Clear()
		break
	case EOT:
		proto.OnSetInitialized(ac, false)

		if proto.SendOrdSegStringQueue.Len() > 0 {
			ASTM_prevSeqNo = 0
			go func() { communicationCheckPackagesToSendChannel <- true }()
			//proto.SendString(ac, string(ENQ))
		} else {
			proto.SendString(ac, string(ACK))
		}
		break
	default:
		proto.SendString(ac, string(ACK))
		break
	}

	MsgBUffer = proto.ASTMBlocks.SplitBlocksUTF8(MsgBUffer, string(rune(ENQ)), string(rune(EOT)), false)

	go proto.parseASTMBlocks(ac)
}

func (proto *ASTMProtocol) parseASTMBlocks(ac config.AnalyzerConnection) {

	for {
		if proto.ASTMBlocks.Len() <= 0 {
			break
		}
		block, err := proto.ASTMBlocks.Pop()
		if err != nil {
			proto.LogASTMMessage(fmt.Sprintf("An error has occured!\n%v", err), true, "logaerr")
			return
		}
		proto.LogASTMMessage(fmt.Sprintf("I have a new ASTM block:\n\n%q\n\n", block), true, "logamsg")

		if proto.CustomBlockDecoder != nil {
			proto.CustomBlockDecoder(proto, ac, block)
		} else {
			go proto.parseRecord(ac, block)
		}

	}
}

/** END ProtocolHandler interface **/

/** BEGIN IMPLEMENTING ASTM **/

func (proto *ASTMProtocol) CheckControl(data string) int {
	val := 0
	for i := 0; i < len(data); i++ {
		val += int([]rune(data)[i])
	}
	return (val & 255) % 256
}
func (proto *ASTMProtocol) VerifySegment(seg string) (string, string, bool, error) {
	//Expect segment in the form STX[seq][segHead].....<segment line separator: ex \r>ETXxn    (xn - checksum) and I have to loose the last  \rETX too from the final string
	startRune := rune(STX)
	endRune := rune(ETX)
	skipChars := 1
	if proto.CustomEachLineWithoutSequenceNo {
		skipChars = 0
	}
	stxPos := strings.IndexRune(seg, startRune)
	etxPos := strings.IndexRune(seg, endRune)
	if stxPos < 0 || etxPos < 0 || stxPos+2 >= etxPos {
		proto.LogASTMMessage("Invalid ASTM segment - STX, ETX not found", true, "logaerr")
		//Here I have STX[seq][segHead].....\rETXxn    (xn - checksum) and I have to loose the last  \rETX too from the final string
		return "", seg, false, errors.New("Invalid ASTM segment - STX, ETX not found")
	}
	//check ASTM checksum
	remainingChars := ""
	if len(seg) >= etxPos+3 {
		checkSum := strings.ToLower(string(seg[etxPos+1 : etxPos+3])) //Skip ETX and include 2 chars after
		checkString := seg[stxPos+skipChars : etxPos+1]               //Ski[ STX only and include ETX for calculation
		calcCheckSum := strings.ToLower(fmt.Sprintf("%.2x", proto.CheckControl(checkString)))

		if len(seg) >= etxPos+3 {
			remainingChars = seg[etxPos+3:]
		}

		if checkSum != calcCheckSum {
			proto.LogASTMMessage(fmt.Sprintf("Invalid ASTM segment - checksum mismatch %s != %s \n", checkSum, calcCheckSum), true, "logaerr")
			proto.LogASTMMessage(fmt.Sprintf("Invalid ASTM segment - for string %s", checkString), true, "logaerr")
			proto.LogASTMMessage(fmt.Sprintf("Invalid ASTM segment - for byte array %v", []byte(checkString)), true, "logaerr")
			return "", remainingChars, true, errors.New("Invalid ASTM segment - checksum mismatch")
		} else {
			if debugASTMLevel > 3 {
				proto.LogASTMMessage(fmt.Sprintf("Checksum ok %s\n", calcCheckSum), true, "logamsg")
			}
		}
	} else {
		if proto.ValidateChecksumAnyway {
			proto.LogASTMMessage("Invalid ASTM segment Checksum information not found", true, "logaerr")
			return "", remainingChars, true, errors.New("Invalid ASTM segment - information not found")
		}
	}

	return seg[stxPos+1+skipChars : etxPos-1], remainingChars, true, nil
}
func (proto *ASTMProtocol) parseRecord(ac config.AnalyzerConnection, data string) {
	//strLines := general.StringQueue{}

	if len(data) > 0 && rune(data[0]) == rune(ENQ) {
		data = data[1:]
	}
	var err error
	//strLines.Split(data, , true)

	if proto.CustomEachLineWithoutSTXETX {
		//I have to clear them here
		data, _, _, err = proto.VerifySegment(data)
		if err != nil {
			proto.LogASTMMessage(fmt.Sprintf("An error has occured while parsing the block!\n%v", err), true, "logaerr")
			proto.SendString(ac, string(EOT))
			return
		}
		proto.LogASTMMessage("\nReceived data verified!", true, "logamsg")
	}

	linesSplitter := ""
	if proto.CustomBlockLinesSplitter != nil {
		linesSplitter = proto.CustomBlockLinesSplitter()
	} else {
		linesSplitter = fmt.Sprintf("%c%c", CR, LF)
	}
	strLines := strings.Split(data, linesSplitter)
	var ASTMRec ASTMSegmentInterface = nil
	strLinesLen := len(strLines)
	proto.LogASTMMessage(fmt.Sprintf("\nstrLines len: %d", strLinesLen), true, "logamsg")
	var i int
	prefixString := ""
	hasSTXETX := false
	for i = 0; i < strLinesLen; i++ {
		idx_SegName := 0
		ASTMRec = nil

		tmpStr := prefixString + strLines[i]
		proto.LogASTMMessage(fmt.Sprintf("\nNext segment: [%d/%d] - %q \n as byte arr %v ", i, strLinesLen, tmpStr), true, "logamsg")
		proto.LogASTMMessage(fmt.Sprintf("\nNext segment as byte array: %v ", []byte(tmpStr)), true, "logamsg")
		if prefixString != "" {
			proto.LogASTMMessage(fmt.Sprintf("\nHaving prefix %q", prefixString), true, "logamsg")
		}

		if tmpStr == "" || tmpStr == string(EOT) {
			proto.LogASTMMessage(fmt.Sprintf("\nSkipping %q", tmpStr), true, "logamsg")
			continue
		}
		//proto.LogASTMMessage("\nVerifying segment", true, "logamsg")
		if !proto.CustomEachLineWithoutSTXETX {
			tmpStr, prefixString, hasSTXETX, err = proto.VerifySegment(tmpStr)
			//In case I don't have a complete line - I will try to reconstruct it therefore adding it to the next one
			if !hasSTXETX {
				//this means that here we have received an  linesplitter in a segment and we have to put it back to have the correct checksum in the end
				prefixString += linesSplitter
				continue
			}

			if err != nil {
				proto.LogASTMMessage(fmt.Sprintf("An error has occured!\n%v", err), true, "logaerr")
				proto.SendString(ac, string(EOT))
				return
			}
			//proto.LogASTMMessage(fmt.Sprintf("\nSegment verified: %q", tmpStr), true, "logamsg")
		}

		segType := strings.Trim(string(tmpStr[idx_SegName:1]), " ")

		seg := ASTMSegment{FieldSeparator: "|"}
		switch segType {
		case "H":
			ASTMRec = &ASTMHeaderSegment{Seg: seg}
			break
		case "P":
			ASTMRec = &ASTMPIDSegment{Seg: seg}
			break
		case "O":
			ASTMRec = &ASTMTORSegment{Seg: seg}
			break
		case "Q":
			ASTMRec = &ASTMRIRSegment{Seg: seg}
			break
		case "R":
			ASTMRec = &ASTMResRecSegment{Seg: seg}
			break
		case "L":
			ASTMRec = &ASTMMTRSegment{Seg: seg}
			break
		case "M":
			ASTMRec = &ASTMMIRSegment{Seg: seg}
			break
		}
		if ASTMRec != nil {
			ASTMRec.ParseASTMSegmentFromString(tmpStr)
			if proto.CustomReceiveSegment != nil {
				err = proto.CustomReceiveSegment(proto, ASTMRec)
			} else {
				err = proto.ReceiveSegment(ASTMRec)
			}
			if err != nil {
				proto.LogASTMMessage(fmt.Sprintf("I have an error from receive segment ", err), true, "logaerr")
				proto.SendString(ac, string(EOT))
				return
			}
		}
	}
	proto.LogASTMMessage(fmt.Sprintf("\nFor routine finished at %d/%d", i, strLinesLen), true, "logamsg")
}

func (proto *ASTMProtocol) ReceiveHeaderSegment(ASTMRec *ASTMHeaderSegment) error {
	if proto.CFGRestrictToASTMVer != "" && ASTMRec.ASTMVer != proto.CFGRestrictToASTMVer {
		proto.LogASTMMessage(fmt.Sprintf("Failed ASTM Security check: Received ASTM version %s is not the expected one %s", ASTMRec.ASTMVer, proto.CFGRestrictToASTMVer), true, "logaerr")
		return errors.New(fmt.Sprintf("Failed ASTM Security check: Received ASTM version %s is not the expected one %s", ASTMRec.ASTMVer, proto.CFGRestrictToASTMVer))
	}
	if proto.CFGBlockUnknownPassword != "" && ASTMRec.AccessPassword != proto.CFGBlockUnknownPassword {
		proto.LogASTMMessage("Failed ASTM Security check: Wrong access password.", true, "logaerr")
		return errors.New("Failed ASTM Security check: Wrong access password.")
	}
	if proto.CFGBlockUnknownSerial != "" {
		fp := FieldParser{GetFromField: "SenderName", GetFromFieldIdx: 4, ReturnType: "string", SplitFieldBy: "^", GetIdFromSplitIdx: 2}
		serNo := fp.TryToParse(ASTMRec)
		serNoStr, ok := serNo.(string)
		if ok {
			if serNoStr != proto.CFGBlockUnknownSerial {
				proto.LogASTMMessage(fmt.Sprintf("Failed ASTM Security check: Given serial %s differs from the expected one %s.", serNoStr, proto.CFGBlockUnknownSerial), true, "logaerr")
				return errors.New(fmt.Sprintf("Failed ASTM Security check: Given serial %s differs from the expected one %s.", serNoStr, proto.CFGBlockUnknownSerial))
			}
		} else {
			proto.LogASTMMessage("Failed ASTM Security check: Cannot get a serial for the analyzer", true, "logaerr")
			return errors.New("Failed ASTM Security check: Cannot get a serial for the analyzer")
		}

	}
	return nil
}
func (proto *ASTMProtocol) ReceivePatientSegment(ASTMRec *ASTMPIDSegment) error {
	if proto.HasValidPatient() {
		proto.ResultsFilesQueue.Push(proto.FileData)
		proto.FileData = sqlitewrapper.SQLOrder{}
		go proto.SaveResultToDatabase()
	}

	proto.FileData = sqlitewrapper.SQLOrder{ResultType: sqlitewrapper.ResultPatient}

	if len(proto.CFGPatientIDParser) <= 0 {
		proto.CFGPatientIDParser = DefaultPatientIDParser
	}

	for _, parser := range proto.CFGPatientIDParser {
		tmpVal := parser.TryToParse(ASTMRec)
		tmpValInt, ok := tmpVal.(int)
		if ok && tmpValInt > 0 {
			proto.FileData.PatientId = strconv.Itoa(tmpValInt)
			break
		}
	}
	proto.FileData.PatientName = ASTMRec.Name

	return nil
}
func (proto *ASTMProtocol) ParseOrderSegmentsToASTMPackages() error {
	for proto.OrderReqestsQueue.Len() > 0 {
		tmpObj, err := proto.OrderReqestsQueue.Pop()
		if err != nil {
			proto.LogASTMMessage(fmt.Sprintf("Error on respond to analyzer query: %v\n", err), true, "logaerr")
			return err
		}
		tmpFile, ok := tmpObj.(sqlitewrapper.SQLOrder)
		if !ok {
			proto.LogASTMMessage("Patient conversion error", true, "logaerr")
			return errors.New("Patient conversion error")
		}

		nowt := time.Now()
		//first load order from DB

		head := ASTMHeaderSegment{
			Delimiter:        proto.lastHeadSegment.Delimiter, //"\^&",
			MessageControlID: "1",
			AccessPassword:   proto.lastHeadSegment.AccessPassword,
			SenderName:       proto.lastHeadSegment.ReceiverID,
			ReceiverID:       proto.lastHeadSegment.SenderName,
			CommentSI:        "",
			Processing:       "P",
			ASTMVer:          proto.lastHeadSegment.ASTMVer, //"E1394-97",
			DateAndTime:      nowt.Format("20060102150405"),
		}
		head.Create()
		headerSegment := proto.BuildHeaderRecord(head)
		patient := ASTMPIDSegment{
			PracticePatID: "",
			LabPatID:      tmpFile.FileId,
			Name:          tmpFile.PatientName,
			Sex:           tmpFile.PatientSex,
			BirthDate:     tmpFile.PatientBirthdate,
		}
		patient.Create()
		patientSegment := proto.BuildPatientRecord(patient)

		orderStrs := []string{headerSegment, patientSegment}
		orderSegments := []ASTMTORSegment{}
		for _, tst := range tmpFile.Tests {
			order := ASTMTORSegment{
				SampleID:          proto.lastOrderInquirySegment.StartingRangeID,
				SequenceNo:        "1",
				UniversalTestID:   tst.Code,
				Priority:          "R",
				RequestedDateTime: nowt.Format("20060102150405"),
				ActionCode:        "N",
				SpecimenType:      "1",
				RecordType:        "O",
			}
			order.Create()
			orderSegments = append(orderSegments, order)
		}

		orderSegmentsStrs := proto.BuildTestOrderRecord(orderSegments)
		for _, orderSegment := range orderSegmentsStrs {
			fmt.Printf("\nOrder seg:\n%q", orderSegment)
			orderStrs = append(orderStrs, orderSegment)
		}

		termination := ASTMMTRSegment{
			SequenceNo: "1",
			TermCode:   "N",
		}
		termination.Create()
		terminationSegment := proto.BuildMessageTerminatorRecord(termination)
		orderStrs = append(orderStrs, terminationSegment)

		proto.SendOrdSegStringQueue.PushArr(orderStrs)

		go func() { communicationCheckPackagesToSendChannel <- true }()
	}
	return nil

}
func (proto *ASTMProtocol) ReceiveQuerySegment(ASTMRec *ASTMRIRSegment) error {
	go func() {
		if proto.HasValidOrder() {
			proto.OrderReqestsQueue.Push(proto.OrderData)
		}
		proto.OrderData = sqlitewrapper.SQLOrder{} //ready for a new one

		strLines := general.StringQueue{}
		strLines.Split(ASTMRec.StartingRangeID, "^", true) //should have smth like 000001^01^         619138^B
		if strLines.Len() > 3 {
			proto.OrderData.FileId = strings.TrimSpace(strLines.GetStringOrVoid(2))
			proto.OrderData.Tests = []sqlitewrapper.SQLTest{}

			fileId, err := strconv.Atoi(proto.OrderData.FileId)
			if err != nil || fileId <= 0 {
				proto.LogASTMMessage(fmt.Sprintf("\nInvalid File ID %s from segment %s", proto.OrderData.FileId, ASTMRec.StartingRangeID), true, "logaerr")
				return
			}
			nowt := time.Now()
			//first load order from DB

			readerOrderSQLITE, _, err := wisemed.LoadFileFromWMAsObj(nowt.Format("2006-01-02"), -1, -1, -1, fileId)
			if err != nil {
				proto.LogASTMMessage(fmt.Sprintf("An error has occured!\n%v", err), true, "logaerr")
				return
			}
			readerOrderSQLITE.FormatDatesForDB()
			proto.LogASTMMessage(fmt.Sprintf("\nFile loaded from WM:\n%d name: %s", readerOrderSQLITE.FileId, readerOrderSQLITE.PatientName), true, "logamsg")
			proto.LogASTMMessage("\nTests on file:", true, "logamsg")
			//creating a package to send to the analyzer
			for tstIdx, tst := range readerOrderSQLITE.Tests {
				proto.LogASTMMessage(fmt.Sprintf("\n%d - %s [%s] (%s);", tstIdx, tst.Name, tst.Code, tst.Tag), true, "logamsg")
				proto.OrderData.Tests = append(proto.OrderData.Tests, tst)
			}

		}

		proto.OrderReqestsQueue.Push(proto.OrderData)
		proto.OrderData = sqlitewrapper.SQLOrder{}
		proto.OrderData.Tests = nil
		go proto.ParseOrderSegmentsToASTMPackages()
	}()

	return nil
}

func (proto *ASTMProtocol) ReceiveOrderSegment(ASTMRec *ASTMTORSegment) error {
	if proto.FileData.FileId != "0" && proto.FileData.FileId != "" {
		return nil
	}

	if len(proto.CFGFileIDParser) <= 0 {
		proto.CFGFileIDParser = DefaultFileIDParser
	}

	for _, parser := range proto.CFGFileIDParser {
		tmpVal := parser.TryToParse(ASTMRec)
		tmpValInt, ok := tmpVal.(int)
		if ok && tmpValInt > 0 {
			proto.FileData.FileId = strconv.Itoa(tmpValInt)
			break
		}
	}

	if proto.CustomISQcDecoder != nil {
		proto.FileData.ResultType = proto.CustomISQcDecoder(proto, ASTMRec)
	} else {
		strLines := general.StringQueue{}
		strLines.Split(ASTMRec.InstrSpecimenID, "^", true)

		if strLines.Len() >= 5 {
			if strings.Contains(strLines.GetStringOrVoid(4), "CONTROL") {
				proto.FileData.ResultType = sqlitewrapper.ResultQC
			} else {
				if strings.TrimSpace(ASTMRec.ActionCode) == "Q" {
					proto.FileData.ResultType = sqlitewrapper.ResultQC
				}
			}
		}
	}
	return nil
}
func (proto *ASTMProtocol) ReceiveResultSegment(ASTMRec *ASTMResRecSegment) error {
	if proto.FileData.FileId == "" || proto.FileData.FileId == "0" {
		return nil
	}
	strLines := general.StringQueue{}
	strLines.Split(ASTMRec.UniversalTestID, "^", true)

	testCode := strings.Replace(ASTMRec.UniversalTestID, "^", "", -1)

	if strLines.Len() > 4 {
		testCode = strLines.GetStringOrVoid(4)
	} else {
		if strLines.Len() > 3 {
			testCode = strLines.GetStringOrVoid(3)
		}
	}
	testCode = strings.TrimSpace(testCode)
	tmpAn := sqlitewrapper.SQLTest{Code: testCode, Raw: strings.TrimSpace(ASTMRec.DataValue)}
	proto.FileData.Tests = append(proto.FileData.Tests, tmpAn)

	return nil
}
func (proto *ASTMProtocol) ReceiveFinalSegment(ASTMRec *ASTMMTRSegment) error {
	if proto.HasValidPatient() {
		proto.ResultsFilesQueue.Push(proto.FileData)
		//FOR TESTS ! :D
		proto.FileData = sqlitewrapper.SQLOrder{} //ready for a new one
		go proto.SaveResultToDatabase()
	} else {
		proto.LogASTMMessage("\n\nATTENETION!!! On Final segment I have an invalid patient\n\n", true, "logamsg")
	}
	return nil
}

func (proto *ASTMProtocol) SaveResultToDatabase() error {
	for proto.ResultsFilesQueue.Len() > 0 {
		tmpObj, err := proto.ResultsFilesQueue.Pop()
		if err != nil {
			proto.LogASTMMessage(fmt.Sprintf("Erroron save result: %v\n", err), true, "logaerr")
			return err
		}
		tmpFile, ok := tmpObj.(sqlitewrapper.SQLOrder)
		if !ok {
			proto.LogASTMMessage("Patient conversion error", true, "logaerr")
			return errors.New("Patient conversion error")
		}

		nowt := time.Now()
		//first load order from DB

		fileId, err := strconv.Atoi(tmpFile.FileId)
		if err != nil || fileId <= 0 {
			proto.LogASTMMessage("FileID is not a positive integer - aborting the save", true, "logaerr")
			continue
		}

		readerOrderSQLITE, _, err := wisemed.LoadFileFromWMAsObj(nowt.Format("2006-01-02"), -1, -1, -1, fileId)
		if err != nil {
			return err
		}
		readerOrderSQLITE.FormatDatesForDB()
		proto.LogASTMMessage(fmt.Sprintf("\nFile loaded from WM:\n%d name: %s", readerOrderSQLITE.FileId, readerOrderSQLITE.PatientName), true, "logamsg")
		proto.LogASTMMessage(fmt.Sprintf("\nTests on file (save):"), true, "logamsg")

		for tstIdx, tst := range readerOrderSQLITE.Tests {
			proto.LogASTMMessage(fmt.Sprintf("\n%d - %s [%s] (%s);", tstIdx, tst.Name, tst.Code, tst.Tag), true, "logamsg")
		}
		someDataChanged := false

		for _, test := range tmpFile.Tests {
			proto.LogASTMMessage(fmt.Sprintf("Searching for test: %s, ", test.Code), true, "logamsg")
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
				proto.LogASTMMessage("Not found - adding as analyzer test", true, "logamsg")
			} else {
				proto.LogASTMMessage("Found", true, "logamsg")
			}
		}
		if someDataChanged {
			err = readerOrderSQLITE.SaveFromCommToDB()
			if err != nil {
				proto.LogASTMMessage(fmt.Sprintf("I have an error on saving to database:\n %q", err), true, "logaerr")
				return err
			}
		}

	}

	return nil
}

func (proto *ASTMProtocol) ReceiveSegment(ASTMRec ASTMSegmentInterface) error {
	segmentName := ASTMRec.GetASTMSegmentField(0)
	proto.LogASTMMessage(fmt.Sprintf("\nReceive %s segment", segmentName), true, "logamsg")
	switch segmentName {
	case "H":
		seg, ok := ASTMRec.(*ASTMHeaderSegment)
		proto.lastHeadSegment = *seg
		if !ok {
			proto.LogASTMMessage("Could not convert ASTMHeaderSegment", true, "logaerr")
			return errors.New("Could not convert ASTMHeaderSegment")
		}
		if proto.CustomReceiveHeaderSegment != nil {
			return proto.CustomReceiveHeaderSegment(proto, seg)
		} else {
			return proto.ReceiveHeaderSegment(seg)
		}
		break
	case "P":
		seg, ok := ASTMRec.(*ASTMPIDSegment)
		if !ok {
			proto.LogASTMMessage("Could not convert ASTMPIDSegment", true, "logaerr")
			return errors.New("Could not convert ASTMPIDSegment")
		}
		if proto.CustomReceivePatientSegment != nil {
			return proto.CustomReceivePatientSegment(proto, seg)
		} else {
			return proto.ReceivePatientSegment(seg)
		}
		break
	case "Q":
		seg, ok := ASTMRec.(*ASTMRIRSegment)
		proto.lastOrderInquirySegment = *seg
		if !ok {
			proto.LogASTMMessage("Could not convert ASTMRIRSegment", true, "logaerr")
			return errors.New("Could not convert ASTMRIRSegment")
		}
		if proto.CustomReceiveQuerySegment != nil {
			return proto.CustomReceiveQuerySegment(proto, seg)
		} else {
			return proto.ReceiveQuerySegment(seg)
		}
		break
	case "O":
		seg, ok := ASTMRec.(*ASTMTORSegment)
		if !ok {
			proto.LogASTMMessage("Could not convert ASTMTORSegment", true, "logaerr")
			return errors.New("Could not convert ASTMTORSegment")
		}
		if proto.CustomReceiveOrderSegment != nil {
			return proto.CustomReceiveOrderSegment(proto, seg)
		} else {
			return proto.ReceiveOrderSegment(seg)
		}
		break
	case "R":
		seg, ok := ASTMRec.(*ASTMResRecSegment)
		if !ok {
			proto.LogASTMMessage("Could not convert ASTMResRecSegment", true, "logaerr")
			return errors.New("Could not convert ASTMResRecSegment")
		}
		if proto.CustomReceiveResultSegment != nil {
			return proto.CustomReceiveResultSegment(proto, seg)
		} else {
			return proto.ReceiveResultSegment(seg)
		}
		break
	case "L":
		seg, ok := ASTMRec.(*ASTMMTRSegment)
		if !ok {
			proto.LogASTMMessage("Could not convert ASTMMTRSegment", true, "logaerr")
			return errors.New("Could not convert ASTMMTRSegment")
		}
		if proto.CustomReceiveFinalSegment != nil {
			return proto.CustomReceiveFinalSegment(proto, seg)
		} else {
			return proto.ReceiveFinalSegment(seg)
		}
		break
	}
	return nil
}

func (proto *ASTMProtocol) ReceiveOrderRequestAll() {

}
func (proto *ASTMProtocol) SendOrdSegAll() {

}

func (proto *ASTMProtocol) BuildHeaderRecord(ASTMRec ASTMHeaderSegment) string {
	if proto.CustomBuildHeaderRecord != nil {
		return proto.CustomBuildHeaderRecord(proto, ASTMRec)
	}
	return ASTMRec.GetASTMSegment()
}
func (proto *ASTMProtocol) BuildPatientRecord(ASTMRec ASTMPIDSegment) string {
	if proto.CustomBuildPatientRecord != nil {
		return proto.CustomBuildPatientRecord(proto, ASTMRec)
	}
	return ASTMRec.GetASTMSegment()
}
func (proto *ASTMProtocol) BuildTestOrderRecord(ASTMRecords []ASTMTORSegment) []string {
	if proto.CustomBuildTestOrderRecord != nil {
		return proto.CustomBuildTestOrderRecord(proto, ASTMRecords)
	}

	res := []string{}
	for _, ASTMRec := range ASTMRecords {
		ASTMRec.UniversalTestID = fmt.Sprintf("^^^%s^", ASTMRec.UniversalTestID) //^^^TestCode^TestName
		res = append(res, ASTMRec.GetASTMSegment())
	}
	return res
}
func (proto *ASTMProtocol) BuildMessageTerminatorRecord(ASTMRec ASTMMTRSegment) string {
	if proto.CustomBuildMessageTerminatorRecord != nil {
		return proto.CustomBuildMessageTerminatorRecord(proto, ASTMRec)
	}
	return ASTMRec.GetASTMSegment()
}
