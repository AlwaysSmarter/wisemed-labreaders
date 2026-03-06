package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type CreateProtocolHandler func() ProtocolHandler
type IntString string

type WSMessageBroadcaster interface {
	BroadcastWSMessage(ac AnalyzerConnection, msgType string, data string)
}

func ReturnIntOrZero(val string) int {
	intVal, err := strconv.Atoi(val)
	if err != nil {
		intVal = 0
	}
	return intVal
}

func TrimAndRemoveLeadingZeros(val string) string {
	val = strings.TrimSpace(val)
	noZeroVal := strings.TrimLeft(val, " 0")
	if noZeroVal == "" && val != "" {
		noZeroVal = "0"
	}
	return noZeroVal
}

func (st *IntString) UnmarshalJSON(b []byte) error {
	//convert the bytes into an interface
	//this will help us check the type of our value
	//if it is a string that can be converted into an int we convert it
	///otherwise we return an error
	var item interface{}
	if err := json.Unmarshal(b, &item); err != nil {
		return err
	}
	switch v := item.(type) {
	case string:
		fmt.Println("s", v)
		*st = IntString(v)
	case float64:
		fmt.Println("f", v)
		i := fmt.Sprintf("%.f", v)
		*st = IntString(i)
	case float32:
		fmt.Println("f", v)
		i := fmt.Sprintf("%.f", v)
		*st = IntString(i)
	case int:
		fmt.Println("i", v)
		///here convert the string into
		///an integer
		i := strconv.Itoa(v)
		*st = IntString(i)

	}
	return nil
}
