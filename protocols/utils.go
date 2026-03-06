package protocols

import (
	"fmt"
	"strings"
)

type Protocol interface {
	ParseCluster()
	DataArrived()
	ReadData()
}

func SubStr(input string, start int, length int) string {
	asRunes := []rune(input)

	if start >= len(asRunes) {
		return ""
	}

	if start+length > len(asRunes) {
		length = len(asRunes) - start
	}

	return string(asRunes[start : start+length])
}

func StrLen(input string) int {
	return len([]rune(input))
}

func CharAt(input string, index int) string {
	asRunes := []rune(input)

	if index < 0 || index >= len(asRunes) {
		return ""
	}

	return string(asRunes[index])
}

func RuneAt(input string, index int) rune {
	asRunes := []rune(input)

	if index < 0 || index >= len(asRunes) {
		return 0
	}

	return asRunes[index]
}

func GetPackage(msgBUffer *string, prefix string, suffix string, includePrefix bool, includeSuffix bool) string {
	prefixIdx := strings.Index(*msgBUffer, prefix) //start char
	if prefixIdx >= 0 {
		tmpBuffer := *msgBUffer
		*msgBUffer = tmpBuffer[prefixIdx:]
		prefixIdx = 0
	}

	tmpBuffer := *msgBUffer
	if !includePrefix {
		tmpBuffer = tmpBuffer[len(prefix):]
	}
	var suffixIdx int
	if prefixIdx >= 0 {
		//just in case suffix might be a part of the prefix we skip the prefix on chekcking
		suffixIdx = strings.Index(tmpBuffer[len(prefix):], suffix)
		if suffixIdx >= 0 {
			//suffix found - it's real position includes the prefix too
			suffixIdx += len(prefix)
		}
	}
	responseBlock := ""
	fmt.Println("suffixIdx", suffixIdx)
	if suffixIdx >= 0 && prefixIdx >= 0 {
		if includeSuffix {
			responseBlock = tmpBuffer[:suffixIdx+len(suffix)]
		} else {
			if suffixIdx > 0 {
				responseBlock = tmpBuffer[:suffixIdx-1]
			} else {
				responseBlock = ""
			}

		}

		if suffixIdx+len(suffix) < len(tmpBuffer) {
			*msgBUffer = tmpBuffer[suffixIdx+len(suffix):]
		} else {
			if suffixIdx+len(suffix) == len(tmpBuffer) {
				*msgBUffer = ""
			}
		}
	}

	return responseBlock
}

func SaveOrderResults() error {
	//load order from DB
	//orderRec, _, err := wisemed.LoadFileFromWMAsObj(nowt.Format("2006-01-02"), -1, -1, -1, fileId)
	//if err != nil {
	//	return err
	//}
	return nil
}
