package hl7

var HL7_prevSeqNo int = 0

const ENQ = rune(5)
const SOH = rune(1)
const STX = rune(2)
const ETX = rune(3)
const EOT = rune(4)
const ACK = rune(6)
const NAK = rune(21)
const ETB = rune(23)

const LF = rune(10)
const VT = rune(11)
const CR = rune(13)
const FS = rune(28)
const SP = rune(32)

var HL7SegmentTypes = map[string]string{
	"ECD": "Equipment Command Segment 2 EQU Equipment Detail Segment",
	"INV": "Inventory Detail Segment",
	"MSA": "Message Acknowledgment Segment",
	"MSH": "Message Header Segment",
	"NTE": "Comment Segment",
	"OBR": "Observation Request Segment",
	"OBX": "Observation/Result Segment",
	"PID": "Patient Identification Segment",
	"QPD": "Query Parameter Definition Segment",
	"RCP": "Response Control Parameter Segment",
	"SAC": "Specimen Container Detail Segment",
	"SPM": "Specimen Segment",
	"TCD": "Test Code Detail Segment",
	"TQ1": "Timing/Quantity Segment",
	"ORC": "Common Order",
	"ERR": "Error Segment",
}

var HL7SampleTypes = map[string]string{
	"SERPLAS":  "99ROC",
	"UR":       "HL70487",
	"CSF":      "HL70487",
	"SUPN":     "99ROC",
	"FLD":      "HL70487",
	"WB":       "HL70487",
	"SAL":      "HL70487",
	"HEML":     "99ROC",
	"AMN":      "HL70487",
	"PROC_STL": "99ROC",
	"PLAS":     "HL70487",
	"SER":      "HL70487",
	"ORH":      "HL70487",
}
var HL7SampleContainerTypes = map[string]string{
	"SC":   "99ROC",
	"MC":   "99ROC",
	"NST0": "99ROC",
	"FBT1": "99ROC",
	"FBT2": "99ROC",
	"FBT3": "99ROC",
}

var HL7CommonOrder = map[string]interface{}{
	"ORC": []interface{}{
		map[string]interface{}{
			"TQ1": []interface{}{
				map[string]string{
					"OBR": "[TCD]",
				},
			},
		},
	},
}
var HL7RepeatedCommonOrder = map[string]interface{}{
	"{}": []interface{}{HL7CommonOrder},
}

/*
func (fp FieldParser) TryToParse(ASTSeg HL7SegmentInterface) interface{} {
	//first get the data from segment
	segData := ASTSeg.GetHL7SegmentField(fp.GetFromFieldIdx)

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
*/
