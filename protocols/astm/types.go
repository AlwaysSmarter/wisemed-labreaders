package astm

import (
	"fmt"
	"strconv"
	"strings"
	"wisemed-labreaders/config"
	"wisemed-labreaders/general"
)

var ASTM_prevSeqNo int = 0

const ENQ = rune(5)
const SOH = rune(1)
const STX = rune(2)
const ETX = rune(3)
const EOT = rune(4)
const ACK = rune(6)
const NAK = rune(21)
const ETB = rune(23)

const LF = rune(10)
const CR = rune(13)
const SP = rune(32)

type ASTMSegmentInterface interface {
	Create()
	GetASTMSegment(maxFields ...int) string
	ParseASTMSegmentFromString(astmStr string)
	CheckControl(data string)
	GetASTMSegmentField(Idx int) string
	SetASTMSegmentField(Idx int, Val string)
}

type ASTMSegment struct {
	PacketID            int
	FieldsNo            int
	FieldSeparator      string
	SegmentName         string
	EncChars            string
	GetASTMSegmentField func(i int) string
	SetASTMSegmentField func(idx int, val string)
}

func (as *ASTMSegment) GetASTMSegment(maxFields ...int) string {
	ASTM_prevSeqNo++
	if ASTM_prevSeqNo > 7 {
		ASTM_prevSeqNo = 0
	}
	str := ""
	fieldsNo := as.FieldsNo
	if len(maxFields) > 0 {
		fieldsNo = maxFields[0]
	}
	for i := 0; i <= fieldsNo; i++ {
		if str != "" {
			str += as.FieldSeparator
		}
		str += as.GetASTMSegmentField(i)
	}
	str = fmt.Sprintf("%s%s%c%c", strconv.Itoa(ASTM_prevSeqNo), str, CR, ETX)
	return fmt.Sprintf("%c%s%.2x%c%c", STX, str, as.CheckControl(str), CR, LF)
}

func (as *ASTMSegment) ParseASTMSegmentFromString(astmStr string) {
	split := strings.Split(astmStr, as.FieldSeparator)

	for i := 0; i < len(split); i++ {
		as.SetASTMSegmentField(i, split[i])
	}
}
func (as *ASTMSegment) CheckControl(data string) int {
	val := 0
	for i := 0; i < len(data); i++ {
		val += int([]rune(data)[i])
	}
	return (val & 255) % 256
}

type ASTMHeaderSegment struct {
	Seg                   ASTMSegment
	Delimiter             string
	MessageControlID      string
	AccessPassword        string
	SenderName            string
	SenderStrAddr         string
	ReservedField         string
	SenderPhoneNo         string
	SenderCharacteristics string
	ReceiverID            string
	CommentSI             string
	Processing            string
	ASTMVer               string
	DateAndTime           string
}

func (as *ASTMHeaderSegment) Create() {
	as.Seg = ASTMSegment{
		FieldsNo:            13,
		FieldSeparator:      "|",
		SegmentName:         "H",
		GetASTMSegmentField: as.GetASTMSegmentField,
		SetASTMSegmentField: as.SetASTMSegmentField,
	}
}
func (as *ASTMHeaderSegment) SetASTMSegmentField(Idx int, Val string) {
	switch Idx {
	case 0:
		as.Seg.SegmentName = Val
		break
	case 1:
		as.Delimiter = Val
		break
	case 2:
		as.MessageControlID = Val
		break
	case 3:
		as.AccessPassword = Val
		break
	case 4:
		as.SenderName = Val
		break
	case 5:
		as.SenderStrAddr = Val
		break
	case 6:
		as.ReservedField = Val
		break
	case 7:
		as.SenderPhoneNo = Val
		break
	case 8:
		as.SenderCharacteristics = Val
		break
	case 9:
		as.ReceiverID = Val
		break
	case 10:
		as.CommentSI = Val
		break
	case 11:
		as.Processing = Val
		break
	case 12:
		as.ASTMVer = Val
		break
	case 13:
		as.DateAndTime = Val
		break
	}
}
func (as *ASTMHeaderSegment) GetASTMSegmentField(Idx int) string {
	switch Idx {
	case 0:
		return as.Seg.SegmentName
		break
	case 1:
		return as.Delimiter
		break
	case 2:
		return as.MessageControlID
		break
	case 3:
		return as.AccessPassword
		break
	case 4:
		return as.SenderName
		break
	case 5:
		return as.SenderStrAddr
		break
	case 6:
		return as.ReservedField
		break
	case 7:
		return as.SenderPhoneNo
		break
	case 8:
		return as.SenderCharacteristics
		break
	case 9:
		return as.ReceiverID
		break
	case 10:
		return as.CommentSI
		break
	case 11:
		return as.Processing
		break
	case 12:
		return as.ASTMVer
		break
	case 13:
		return as.DateAndTime
	}
	return ""
}
func (as *ASTMHeaderSegment) GetASTMSegment(maxFields ...int) string {
	ASTM_prevSeqNo++
	if ASTM_prevSeqNo > 7 {
		ASTM_prevSeqNo = 0
	}
	str := ""
	fieldsNo := as.Seg.FieldsNo
	if len(maxFields) > 0 {
		fieldsNo = maxFields[0]
	}
	for i := 0; i <= fieldsNo; i++ {
		if str != "" {
			str += as.Seg.FieldSeparator
		}
		str += as.GetASTMSegmentField(i)
	}
	str = fmt.Sprintf("%s%s%c%c", strconv.Itoa(ASTM_prevSeqNo), str, CR, ETX)
	return fmt.Sprintf("%c%s%.2x%c%c", STX, str, as.Seg.CheckControl(str), CR, LF)
}
func (as *ASTMHeaderSegment) CheckControl(data string) {
	as.Seg.CheckControl(data)
}
func (as *ASTMHeaderSegment) ParseASTMSegmentFromString(astmStr string) {
	split := strings.Split(astmStr, as.Seg.FieldSeparator)

	for i := 0; i < len(split); i++ {
		as.SetASTMSegmentField(i, split[i])
	}
}

type ASTMPIDSegment struct {
	Seg             ASTMSegment
	SequenceNo      string
	PracticePatID   string
	LabPatID        string
	PatID3          string
	Name            string
	MothersMName    string
	BirthDate       string
	Sex             string
	Race            string
	Address         string
	ResF1           string
	Phone           string
	AttPhisician    string
	SpecField1      string
	SpecField2      string
	PacHeight       string
	PacWeight       string
	KnownDiag       string
	ActiveMed       string
	Diet            string
	PracticeF1      string
	PracticeF2      string
	AdmissionDate   string
	AdmissionStat   string
	Location        string
	NatureOfDiag    string
	AltDiagCode     string
	Religion        string
	MaritalStatus   string
	IsolationStatus string
	Language        string
	HospService     string
	HospInst        string
	DosageCat       string
}

func (as *ASTMPIDSegment) Create() {
	as.Seg.FieldsNo = 34
	as.Seg.FieldSeparator = "|"
	as.Seg.SegmentName = "P"
}
func (as *ASTMPIDSegment) SetASTMSegmentField(Idx int, Val string) {
	switch Idx {
	case 0:
		as.Seg.SegmentName = Val
		break
	case 1:
		as.SequenceNo = Val
		break
	case 2:
		as.PracticePatID = Val
		break
	case 3:
		as.LabPatID = Val
		break
	case 4:
		as.PatID3 = Val
		break
	case 5:
		as.Name = Val
		break
	case 6:
		as.MothersMName = Val
		break
	case 7:
		as.BirthDate = Val
		break
	case 8:
		if len(Val) >= 1 {
			as.Sex = Val[:1]
		} else {
			as.Sex = ""
		}
		break
	case 9:
		as.Race = Val
		break
	case 10:
		as.Address = Val
		break
	case 11:
		as.ResF1 = Val
		break
	case 12:
		as.Phone = Val
		break
	case 13:
		as.AttPhisician = Val
		break
	case 14:
		as.SpecField1 = Val
		break
	case 15:
		as.SpecField2 = Val
		break
	case 16:
		as.PacHeight = Val
		break
	case 17:
		as.PacWeight = Val
		break
	case 18:
		as.KnownDiag = Val
		break
	case 19:
		as.ActiveMed = Val
		break
	case 20:
		as.Diet = Val
		break
	case 21:
		as.PracticeF1 = Val
		break
	case 22:
		as.PracticeF2 = Val
		break
	case 23:
		as.AdmissionDate = Val
		break
	case 24:
		as.AdmissionStat = Val
		break
	case 25:
		as.Location = Val
		break
	case 26:
		as.NatureOfDiag = Val
		break
	case 27:
		as.AltDiagCode = Val
		break
	case 28:
		as.Religion = Val
		break
	case 29:
		as.MaritalStatus = Val
		break
	case 30:
		as.IsolationStatus = Val
		break
	case 31:
		as.Language = Val
		break
	case 32:
		as.HospService = Val
		break
	case 33:
		as.HospInst = Val
		break
	case 34:
		as.DosageCat = Val
	}
}
func (as *ASTMPIDSegment) GetASTMSegmentField(Idx int) string {
	switch Idx {
	case 0:
		return as.Seg.SegmentName
		break
	case 1:
		return as.SequenceNo
		break
	case 2:
		return as.PracticePatID
		break
	case 3:
		return as.LabPatID
		break
	case 4:
		return as.PatID3
		break
	case 5:
		return as.Name
		break
	case 6:
		return as.MothersMName
		break
	case 7:
		return as.BirthDate
		break
	case 8:
		return as.Sex
		break
	case 9:
		return as.Race
		break
	case 10:
		return as.Address
		break
	case 11:
		return as.ResF1
		break
	case 12:
		return as.Phone
		break
	case 13:
		return as.AttPhisician
		break
	case 14:
		return as.SpecField1
		break
	case 15:
		return as.SpecField2
		break
	case 16:
		return as.PacHeight
		break
	case 17:
		return as.PacWeight
		break
	case 18:
		return as.KnownDiag
		break
	case 19:
		return as.ActiveMed
		break
	case 20:
		return as.Diet
		break
	case 21:
		return as.PracticeF1
		break
	case 22:
		return as.PracticeF2
		break
	case 23:
		return as.AdmissionDate
		break
	case 24:
		return as.AdmissionStat
		break
	case 25:
		return as.Location
		break
	case 26:
		return as.NatureOfDiag
		break
	case 27:
		return as.AltDiagCode
		break
	case 28:
		return as.Religion
		break
	case 29:
		return as.MaritalStatus
		break
	case 30:
		return as.IsolationStatus
		break
	case 31:
		return as.Language
		break
	case 32:
		return as.HospService
		break
	case 33:
		return as.HospInst
		break
	case 34:
		return as.DosageCat
	}
	return ""
}
func (as *ASTMPIDSegment) GetASTMSegment(maxFields ...int) string {
	ASTM_prevSeqNo++
	if ASTM_prevSeqNo > 7 {
		ASTM_prevSeqNo = 0
	}
	str := ""
	fieldsNo := as.Seg.FieldsNo
	if len(maxFields) > 0 {
		fieldsNo = maxFields[0]
	}
	for i := 0; i <= fieldsNo; i++ {
		if str != "" {
			str += as.Seg.FieldSeparator
		}
		str += as.GetASTMSegmentField(i)
	}
	str = fmt.Sprintf("%s%s%c%c", strconv.Itoa(ASTM_prevSeqNo), str, CR, ETX)
	return fmt.Sprintf("%c%s%.2x%c%c", STX, str, as.Seg.CheckControl(str), CR, LF)
}
func (as *ASTMPIDSegment) CheckControl(data string) {
	as.Seg.CheckControl(data)
}
func (as *ASTMPIDSegment) ParseASTMSegmentFromString(astmStr string) {
	split := strings.Split(astmStr, as.Seg.FieldSeparator)

	for i := 0; i < len(split); i++ {
		as.SetASTMSegmentField(i, split[i])
	}
}

type ASTMTORSegment struct {
	Seg                      ASTMSegment
	SequenceNo               string
	SampleID                 string
	InstrSpecimenID          string
	UniversalTestID          string
	Priority                 string
	RequestedDateTime        string
	SpecimentCollectDateTime string
	CollectionEndTime        string
	CollectionVol            string
	CollectorID              string
	ActionCode               string
	DangerCode               string
	RelevantClInfo           string
	DateTimeSpecRecvd        string
	SpecimenType             string
	OrderingPhysician        string
	PhysiscianPhone          string
	UserFld1                 string
	UserFld2                 string
	LabFld1                  string
	LabFld2                  string
	DateTimeResRep           string
	InstrumentCharge         string
	InstrumentSectionID      string
	RecordType               string
	ResFld                   string
	LocOfSpecCol             string
	NosocomialInfFlg         string
	SpecimenService          string
	SpecimenInst             string
}

func (as *ASTMTORSegment) Create() {
	as.Seg.FieldsNo = 30
	as.Seg.FieldSeparator = "|"
	as.Seg.SegmentName = "O"
}
func (as *ASTMTORSegment) SetASTMSegmentField(Idx int, Val string) {
	switch Idx {
	case 0:
		as.Seg.SegmentName = Val
		break
	case 1:
		as.SequenceNo = Val
		break
	case 2:
		as.SampleID = Val
		break
	case 3:
		as.InstrSpecimenID = Val
		break
	case 4:
		as.UniversalTestID = Val
		break
	case 5:
		as.Priority = Val
		break
	case 6:
		as.RequestedDateTime = Val
		break
	case 7:
		as.SpecimentCollectDateTime = Val
		break
	case 8:
		as.CollectionEndTime = Val
		break
	case 9:
		as.CollectionVol = Val
		break
	case 10:
		as.CollectorID = Val
		break
	case 11:
		as.ActionCode = Val
		break
	case 12:
		as.DangerCode = Val
		break
	case 13:
		as.RelevantClInfo = Val
		break
	case 14:
		as.DateTimeSpecRecvd = Val
		break
	case 15:
		as.SpecimenType = Val
		break
	case 16:
		as.OrderingPhysician = Val
		break
	case 17:
		as.PhysiscianPhone = Val
		break
	case 18:
		as.UserFld1 = Val
		break
	case 19:
		as.UserFld2 = Val
		break
	case 20:
		as.LabFld1 = Val
		break
	case 21:
		as.LabFld2 = Val
		break
	case 22:
		as.DateTimeResRep = Val
		break
	case 23:
		as.InstrumentCharge = Val
		break
	case 24:
		as.InstrumentSectionID = Val
		break
	case 25:
		as.RecordType = Val
		break
	case 26:
		as.ResFld = Val
		break
	case 27:
		as.LocOfSpecCol = Val
		break
	case 28:
		as.NosocomialInfFlg = Val
		break
	case 29:
		as.SpecimenService = Val
		break
	case 30:
		as.SpecimenInst = Val
	}
}
func (as *ASTMTORSegment) GetASTMSegmentField(Idx int) string {
	switch Idx {
	case 0:
		return as.Seg.SegmentName
		break
	case 1:
		return as.SequenceNo
		break
	case 2:
		return as.SampleID
		break
	case 3:
		return as.InstrSpecimenID
		break
	case 4:
		return as.UniversalTestID
		break
	case 5:
		return as.Priority
		break
	case 6:
		return as.RequestedDateTime
		break
	case 7:
		return as.SpecimentCollectDateTime
		break
	case 8:
		return as.CollectionEndTime
		break
	case 9:
		return as.CollectionVol
		break
	case 10:
		return as.CollectorID
		break
	case 11:
		return as.ActionCode
		break
	case 12:
		return as.DangerCode
		break
	case 13:
		return as.RelevantClInfo
		break
	case 14:
		return as.DateTimeSpecRecvd
		break
	case 15:
		return as.SpecimenType
		break
	case 16:
		return as.OrderingPhysician
		break
	case 17:
		return as.PhysiscianPhone
		break
	case 18:
		return as.UserFld1
		break
	case 19:
		return as.UserFld2
		break
	case 20:
		return as.LabFld1
		break
	case 21:
		return as.LabFld2
		break
	case 22:
		return as.DateTimeResRep
		break
	case 23:
		return as.InstrumentCharge
		break
	case 24:
		return as.InstrumentSectionID
		break
	case 25:
		return as.RecordType
		break
	case 26:
		return as.ResFld
		break
	case 27:
		return as.LocOfSpecCol
		break
	case 28:
		return as.NosocomialInfFlg
		break
	case 29:
		return as.SpecimenService
		break
	case 30:
		return as.SpecimenInst
	}
	return ""
}
func (as *ASTMTORSegment) GetASTMSegment(maxFields ...int) string {
	ASTM_prevSeqNo++
	if ASTM_prevSeqNo > 7 {
		ASTM_prevSeqNo = 0
	}
	str := ""
	fieldsNo := as.Seg.FieldsNo
	if len(maxFields) > 0 {
		fieldsNo = maxFields[0]
	}
	for i := 0; i <= fieldsNo; i++ {
		if str != "" {
			str += as.Seg.FieldSeparator
		}
		str += as.GetASTMSegmentField(i)
	}
	str = fmt.Sprintf("%s%s%c%c", strconv.Itoa(ASTM_prevSeqNo), str, CR, ETX)
	return fmt.Sprintf("%c%s%.2x%c%c", STX, str, as.Seg.CheckControl(str), CR, LF)
}
func (as *ASTMTORSegment) CheckControl(data string) {
	as.Seg.CheckControl(data)
}
func (as *ASTMTORSegment) ParseASTMSegmentFromString(astmStr string) {
	split := strings.Split(astmStr, as.Seg.FieldSeparator)

	for i := 0; i < len(split); i++ {
		as.SetASTMSegmentField(i, split[i])
	}
}

type ASTMResRecSegment struct {
	Seg                 ASTMSegment
	SequenceNo          string
	UniversalTestID     string
	DataValue           string
	UnitsOfMeas         string
	RefRang             string
	Flags               string
	NatureOfAbn         string
	ResStatus           string
	DateOfChange        string
	OperatorID          string
	DateTimeTestStarted string
	DateTimeTestCompl   string
	InstrumentID        string
}

func (as *ASTMResRecSegment) Create() {
	as.Seg.FieldsNo = 13
	as.Seg.FieldSeparator = "|"
	as.Seg.SegmentName = "R"
}
func (as *ASTMResRecSegment) SetASTMSegmentField(Idx int, Val string) {
	switch Idx {
	case 0:
		as.Seg.SegmentName = Val
		break
	case 1:
		as.SequenceNo = Val
		break
	case 2:
		as.UniversalTestID = Val
		break
	case 3:
		as.DataValue = Val
		break
	case 4:
		as.UnitsOfMeas = Val
		break
	case 5:
		as.RefRang = Val
		break
	case 6:
		as.Flags = Val
		break
	case 7:
		as.NatureOfAbn = Val
		break
	case 8:
		as.ResStatus = Val
		break
	case 9:
		as.DateOfChange = Val
		break
	case 10:
		as.OperatorID = Val
		break
	case 11:
		as.DateTimeTestStarted = Val
		break
	case 12:
		as.DateTimeTestCompl = Val
		break
	case 13:
		as.InstrumentID = Val
	}
}
func (as *ASTMResRecSegment) GetASTMSegmentField(Idx int) string {
	switch Idx {
	case 0:
		return as.Seg.SegmentName
		break
	case 1:
		return as.SequenceNo
		break
	case 2:
		return as.UniversalTestID
		break
	case 3:
		return as.DataValue
		break
	case 4:
		return as.UnitsOfMeas
		break
	case 5:
		return as.RefRang
		break
	case 6:
		return as.Flags
		break
	case 7:
		return as.NatureOfAbn
		break
	case 8:
		return as.ResStatus
		break
	case 9:
		return as.DateOfChange
		break
	case 10:
		return as.OperatorID
		break
	case 11:
		return as.DateTimeTestStarted
		break
	case 12:
		return as.DateTimeTestCompl
		break
	case 13:
		return as.InstrumentID
	}
	return ""
}
func (as *ASTMResRecSegment) GetASTMSegment(maxFields ...int) string {
	ASTM_prevSeqNo++
	if ASTM_prevSeqNo > 7 {
		ASTM_prevSeqNo = 0
	}
	str := ""
	fieldsNo := as.Seg.FieldsNo
	if len(maxFields) > 0 {
		fieldsNo = maxFields[0]
	}
	for i := 0; i <= fieldsNo; i++ {
		if str != "" {
			str += as.Seg.FieldSeparator
		}
		str += as.GetASTMSegmentField(i)
	}
	str = fmt.Sprintf("%s%s%c%c", strconv.Itoa(ASTM_prevSeqNo), str, CR, ETX)
	return fmt.Sprintf("%c%s%.2x%c%c", STX, str, as.Seg.CheckControl(str), CR, LF)
}
func (as *ASTMResRecSegment) CheckControl(data string) {
	as.Seg.CheckControl(data)
}
func (as *ASTMResRecSegment) ParseASTMSegmentFromString(astmStr string) {
	split := strings.Split(astmStr, as.Seg.FieldSeparator)

	for i := 0; i < len(split); i++ {
		as.SetASTMSegmentField(i, split[i])
	}
}

type ASTMMTRSegment struct {
	Seg        ASTMSegment
	SequenceNo string
	TermCode   string
}

func (as *ASTMMTRSegment) Create() {
	as.Seg.FieldsNo = 2
	as.Seg.FieldSeparator = "|"
	as.Seg.SegmentName = "L"
}
func (as *ASTMMTRSegment) SetASTMSegmentField(Idx int, Val string) {
	switch Idx {
	case 0:
		as.Seg.SegmentName = Val
		break
	case 1:
		as.SequenceNo = Val
		break
	case 2:
		as.TermCode = Val
	}
}
func (as *ASTMMTRSegment) GetASTMSegmentField(Idx int) string {
	switch Idx {
	case 0:
		return as.Seg.SegmentName
		break
	case 1:
		return as.SequenceNo
		break
	case 2:
		return as.TermCode
	}
	return ""
}
func (as *ASTMMTRSegment) GetASTMSegment(maxFields ...int) string {
	ASTM_prevSeqNo++
	if ASTM_prevSeqNo > 7 {
		ASTM_prevSeqNo = 0
	}
	str := ""
	fieldsNo := as.Seg.FieldsNo
	if len(maxFields) > 0 {
		fieldsNo = maxFields[0]
	}
	for i := 0; i <= fieldsNo; i++ {
		if str != "" {
			str += as.Seg.FieldSeparator
		}
		str += as.GetASTMSegmentField(i)
	}
	str = fmt.Sprintf("%s%s%c%c", strconv.Itoa(ASTM_prevSeqNo), str, CR, ETX)
	return fmt.Sprintf("%c%s%.2x%c%c", STX, str, as.Seg.CheckControl(str), CR, LF)
}
func (as *ASTMMTRSegment) CheckControl(data string) {
	as.Seg.CheckControl(data)
}
func (as *ASTMMTRSegment) ParseASTMSegmentFromString(astmStr string) {
	split := strings.Split(astmStr, as.Seg.FieldSeparator)

	for i := 0; i < len(split); i++ {
		as.SetASTMSegmentField(i, split[i])
	}
}

type ASTMRIRSegment struct {
	Seg               ASTMSegment
	SequenceNo        string
	StartingRangeID   string
	EndingRangeID     string
	UniversalTestId   string
	NatureReqTL       string
	BegReqResDateTime string
	EndReqResDateTime string
	ReqPhysicianName  string
	ReqPhysicianPhone string
	UserFld1          string
	UserFld2          string
	ReqInfoStatus     string
}

func (as *ASTMRIRSegment) Create() {
	as.Seg.FieldsNo = 12
	as.Seg.FieldSeparator = "|"
	as.Seg.SegmentName = "Q"
}
func (as *ASTMRIRSegment) SetASTMSegmentField(Idx int, Val string) {
	switch Idx {
	case 0:
		as.Seg.SegmentName = Val
		break
	case 1:
		as.SequenceNo = Val
		break
	case 2:
		as.StartingRangeID = Val
		break
	case 3:
		as.EndingRangeID = Val
		break
	case 4:
		as.UniversalTestId = Val
		break
	case 5:
		as.NatureReqTL = Val
		break
	case 6:
		as.BegReqResDateTime = Val
		break
	case 7:
		as.EndReqResDateTime = Val
		break
	case 8:
		as.ReqPhysicianName = Val
		break
	case 9:
		as.ReqPhysicianPhone = Val
		break
	case 10:
		as.UserFld1 = Val
		break
	case 11:
		as.UserFld2 = Val
		break
	case 12:
		as.ReqInfoStatus = Val
	}
}
func (as *ASTMRIRSegment) GetASTMSegmentField(Idx int) string {
	switch Idx {
	case 0:
		return as.Seg.SegmentName
		break
	case 1:
		return as.SequenceNo
		break
	case 2:
		return as.StartingRangeID
		break
	case 3:
		return as.EndingRangeID
		break
	case 4:
		return as.UniversalTestId
		break
	case 5:
		return as.NatureReqTL
		break
	case 6:
		return as.BegReqResDateTime
		break
	case 7:
		return as.EndReqResDateTime
		break
	case 8:
		return as.ReqPhysicianName
		break
	case 9:
		return as.ReqPhysicianPhone
		break
	case 10:
		return as.UserFld1
		break
	case 11:
		return as.UserFld2
		break
	case 12:
		return as.ReqInfoStatus
	}
	return ""
}
func (as *ASTMRIRSegment) GetASTMSegment(maxFields ...int) string {
	ASTM_prevSeqNo++
	if ASTM_prevSeqNo > 7 {
		ASTM_prevSeqNo = 0
	}
	str := ""
	fieldsNo := as.Seg.FieldsNo
	if len(maxFields) > 0 {
		fieldsNo = maxFields[0]
	}
	for i := 0; i <= fieldsNo; i++ {
		if str != "" {
			str += as.Seg.FieldSeparator
		}
		str += as.GetASTMSegmentField(i)
	}
	str = fmt.Sprintf("%s%s%c%c", strconv.Itoa(ASTM_prevSeqNo), str, CR, ETX)
	return fmt.Sprintf("%c%s%.2x%c%c", STX, str, as.Seg.CheckControl(str), CR, LF)
}
func (as *ASTMRIRSegment) CheckControl(data string) {
	as.Seg.CheckControl(data)
}
func (as *ASTMRIRSegment) ParseASTMSegmentFromString(astmStr string) {
	split := strings.Split(astmStr, as.Seg.FieldSeparator)

	for i := 0; i < len(split); i++ {
		as.SetASTMSegmentField(i, split[i])
	}
}

type ASTMMIRSegment struct {
	Seg             ASTMSegment
	SequenceNo      string
	InstrumentAlert string
	TestFlags       string
}

func (as *ASTMMIRSegment) Create() {
	as.Seg.FieldsNo = 4
	as.Seg.FieldSeparator = "|"
	as.Seg.SegmentName = "M"

}
func (as *ASTMMIRSegment) SetASTMSegmentField(Idx int, Val string) {
	switch Idx {
	case 0:
		as.Seg.SegmentName = Val
		break
	case 1:
		as.SequenceNo = Val
		break
	case 2:
		as.InstrumentAlert = Val
		break
	case 3:
		as.TestFlags = Val
	}
}
func (as *ASTMMIRSegment) GetASTMSegmentField(Idx int) string {
	switch Idx {
	case 0:
		return as.Seg.SegmentName
		break
	case 1:
		return as.SequenceNo
		break
	case 2:
		return as.InstrumentAlert
		break
	case 3:
		return as.TestFlags
	}
	return ""
}
func (as *ASTMMIRSegment) GetASTMSegment(maxFields ...int) string {
	ASTM_prevSeqNo++
	if ASTM_prevSeqNo > 7 {
		ASTM_prevSeqNo = 0
	}
	str := ""
	fieldsNo := as.Seg.FieldsNo
	if len(maxFields) > 0 {
		fieldsNo = maxFields[0]
	}
	for i := 0; i <= fieldsNo; i++ {
		if str != "" {
			str += as.Seg.FieldSeparator
		}
		str += as.GetASTMSegmentField(i)
	}
	str = fmt.Sprintf("%s%s%c%c", strconv.Itoa(ASTM_prevSeqNo), str, CR, ETX)
	return fmt.Sprintf("%c%s%.2x%c%c", STX, str, as.Seg.CheckControl(str), CR, LF)
}
func (as *ASTMMIRSegment) CheckControl(data string) {
	as.Seg.CheckControl(data)
}
func (as *ASTMMIRSegment) ParseASTMSegmentFromString(astmStr string) {
	split := strings.Split(astmStr, as.Seg.FieldSeparator)

	for i := 0; i < len(split); i++ {
		as.SetASTMSegmentField(i, split[i])
	}
}

type FieldParser struct {
	GetFromField      string
	GetFromFieldIdx   int
	SplitFieldBy      string
	GetIdFromSplitIdx int
	ReturnType        string
}

func (fp FieldParser) TryToParse(ASTSeg ASTMSegmentInterface) interface{} {
	//first get the data from segment
	segData := ASTSeg.GetASTMSegmentField(fp.GetFromFieldIdx)

	//if we have to split it we do that
	if fp.SplitFieldBy != "" {
		strLines := general.StringQueue{}
		strLines.Split(segData, fp.SplitFieldBy, true)
		//if the split lines are enogh to cover the required index to get from we do that
		segData = strLines.GetStringOrVoid(fp.GetIdFromSplitIdx)
	}

	switch fp.ReturnType {
	case "int":
		return config.ReturnIntOrZero(config.TrimAndRemoveLeadingZeros(segData))
		break
	case "str":
	case "string":
		return segData
		break
	}

	return nil
}
