package general

import (
	"encoding/json"
	"github.com/rivo/uniseg"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/exp/slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

import (
	"errors"
	"fmt"
	"log"
)

var __debug = true
var LoggedInUser *WMLRLoginInfo
var Layout = "02-01-06 15:04:05"
var DateLayout = "02-01-06"
var HourLayout = "15:04:05"

var WebLayout = "2006-01-02 15:04:05"
var WebDateLayout = "2006-01-02"

func DBDateFromWeb(date string) string {
	if len(date) > 10 {
		date = date[:10]
	}
	date = strings.ReplaceAll(date, "-", "/")
	date = strings.ReplaceAll(date, ".", "/")

	parseTime, err := time.Parse("02/01/2006", date)
	if err != nil {
		return "0000-00-00"
	}
	return parseTime.Format("2006-01-02")
}
func WebDateFromDB(date string) string {
	if len(date) > 10 {
		date = date[:10]
	}
	date = strings.ReplaceAll(date, "/", "-")
	date = strings.ReplaceAll(date, ".", "-")

	parseTime, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "0000-00-00"
	}
	return parseTime.Format("02/01/2006")
}

func PrettyPrint(forced bool, obj ...interface{}) {
	if forced == true {
		a, _ := json.MarshalIndent(obj, "", "  ")
		log.Println(string(a))
		return
	}

	if __debug == true {
		a, _ := json.MarshalIndent(obj, "", "  ")
		log.Println(string(a))
	}
}

func MonitorFunc(funcName string) func() {
	PrettyPrint(false, "entered", funcName)
	return func() {
		PrettyPrint(false, "exited", funcName)
	}
}

func HashAndSalt(pwd string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.MinCost)
	return string(hash), err
}

func ComparePasswords(hash, pwd string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pwd))
}

func ReturnValueAsType(val interface{}, typeName string) interface{} {
	fmt.Printf("Convert value to %s", typeName)
	switch typeName {
	case "ObjectID":
		switch val.(type) {
		case primitive.ObjectID:
			return val
			break
		default:
			var tmpStr string
			tmpStr = val.(string)
			retVal, _ := primitive.ObjectIDFromHex(tmpStr)
			return retVal
			break
		}
		break
	case "int":
		switch val.(type) {
		case int:
			return val
			break
		case string:
			retVal, _ := strconv.Atoi(val.(string))
			return retVal
			break
		default:
			return val.(int)
			break
		}
		break
	default:
		return val.(string)
		break
	}
	return nil
}

type ObjectQueue struct {
	Data   []interface{}
	Safety *sync.Mutex
}

func (sq *ObjectQueue) CheckSafety() {
	if sq.Safety == nil {
		sq.Safety = &sync.Mutex{}
	}
}
func (sq *ObjectQueue) Clear() {
	sq.Lock()
	sq.Data = make([]interface{}, 0)
	sq.UnLock()
}
func (sq *ObjectQueue) Len() int {
	sq.Lock()
	len := len(sq.Data)
	sq.UnLock()
	return len
}
func (sq *ObjectQueue) Lock() {
	sq.CheckSafety()
	sq.Safety.Lock()
}
func (sq *ObjectQueue) UnLock() {
	sq.CheckSafety()
	sq.Safety.Unlock()
}
func (sq *ObjectQueue) Push(element interface{}) {
	sq.Lock()
	sq.Data = append(sq.Data, element)
	sq.UnLock()
}
func (sq *ObjectQueue) PushWithIdx(element interface{}) int {
	sq.Lock()
	sq.Data = append(sq.Data, element)
	idx := len(sq.Data) - 1
	sq.UnLock()
	return idx
}
func (sq *ObjectQueue) Pop() (interface{}, error) {
	sq.Lock()
	if len(sq.Data) == 0 {
		sq.UnLock()
		return nil, errors.New("Empty list")
	}

	element := sq.Data[0]
	if len(sq.Data) > 1 {
		sq.Data = sq.Data[1:]
	} else {
		sq.Data = make([]interface{}, 0)
	}
	sq.UnLock()
	return element, nil
}
func (sq *ObjectQueue) PopIdx(idx int) (interface{}, error) {
	sq.Lock()
	if len(sq.Data) == 0 {
		sq.UnLock()
		return nil, errors.New("Empty list")
	}

	if idx < 0 || idx >= len(sq.Data) {
		sq.UnLock()
		return nil, errors.New("Index out of range")
	}
	element := sq.Data[idx]

	tmpData := []interface{}{}
	if idx < len(sq.Data) {
		tmpData = sq.Data[idx+1:]
	}
	sq.Data = sq.Data[:idx]
	for _, el := range tmpData {
		sq.Data = append(sq.Data, el)
	}

	sq.UnLock()
	return element, nil
}
func (sq *ObjectQueue) GetObject(idx int) (interface{}, error) {
	sq.Lock()
	if idx < 0 {
		sq.UnLock()
		return nil, errors.New("Negative index")
	}
	if len(sq.Data) == 0 {
		sq.UnLock()
		return nil, errors.New("Empty list")
	}
	if len(sq.Data) <= idx {
		sq.UnLock()
		return nil, errors.New("Over index")
	}
	element := sq.Data[idx]

	sq.UnLock()
	return element, nil
}
func (sq *ObjectQueue) GetObjectOrNil(idx int) interface{} {
	sq.Lock()
	if idx < 0 || len(sq.Data) == 0 || len(sq.Data) <= idx {
		sq.UnLock()
		return nil
	}
	element := sq.Data[idx]

	sq.UnLock()
	return element
}

type StringQueue struct {
	Data   []string
	Safety *sync.Mutex
}

func (sq *StringQueue) CheckSafety() {
	if sq.Safety == nil {
		sq.Safety = &sync.Mutex{}
	}
}
func (sq *StringQueue) Clear() {
	sq.Lock()
	sq.Data = make([]string, 0)
	sq.UnLock()
}
func (sq *StringQueue) Split(source string, separator string, clearBefore bool) {
	sq.Lock()
	if clearBefore {
		sq.Data = make([]string, 0)
	}
	tmpStr := strings.Split(source, separator)
	for _, str := range tmpStr {
		sq.Data = append(sq.Data, str)
	}
	sq.UnLock()
}
func (sq *StringQueue) SplitBlocksUTF8(source string, start string, end string, clearBefore bool) string {
	sq.Lock()
	if clearBefore {
		sq.Data = make([]string, 0)
	}

	gs := uniseg.NewGraphemes(source)
	gs.Reset()

	haveENQ := false
	i := 0
	ii := 0
	var ch string
	for {
		err := gs.Next()
		if !err {
			break
		} else {
			ch = gs.Str()
		}
		from, to := gs.Positions()
		ii += (to - from)
		switch {
		case ch == end:
			sq.Data = append(sq.Data, source[i:ii])
			source = source[ii:]
			haveENQ = false
			i = 0
			ii = 0
		case ch == start:
			haveENQ = true
			i = ii
		default:
			if !haveENQ {
				//ignore everything until i get the start sequence
				i = ii
			}
		}
	}

	sq.UnLock()
	return source
}

func (sq *StringQueue) Len() int {
	sq.Lock()
	len := len(sq.Data)
	sq.UnLock()
	return len
}
func (sq *StringQueue) Lock() {
	sq.CheckSafety()
	sq.Safety.Lock()
}
func (sq *StringQueue) UnLock() {
	sq.CheckSafety()
	sq.Safety.Unlock()
}
func (sq *StringQueue) Push(element string) {
	sq.Lock()
	sq.Data = append(sq.Data, element)
	sq.UnLock()
}
func (sq *StringQueue) PushArr(elementArr []string) {
	sq.Lock()
	for _, element := range elementArr {
		sq.Data = append(sq.Data, element)
	}
	sq.UnLock()
}
func (sq *StringQueue) Pop() (string, error) {
	sq.Lock()
	if len(sq.Data) == 0 {
		sq.UnLock()
		return "", errors.New("Empty list")
	}

	element := sq.Data[0]
	if len(sq.Data) > 1 {
		sq.Data = sq.Data[1:]
	} else {
		sq.Data = make([]string, 0)
	}
	sq.UnLock()
	return element, nil
}
func (sq *StringQueue) GetString(idx int) (string, error) {
	sq.Lock()
	if idx < 0 {
		sq.UnLock()
		return "", errors.New("Negative index")
	}
	if len(sq.Data) == 0 {
		sq.UnLock()
		return "", errors.New("Empty list")
	}
	if len(sq.Data) <= idx {
		sq.UnLock()
		return "", errors.New("Over index")
	}
	element := sq.Data[idx]

	sq.UnLock()
	return element, nil
}
func (sq *StringQueue) GetStringOrVoid(idx int) string {
	sq.Lock()
	if idx < 0 || len(sq.Data) == 0 || len(sq.Data) <= idx {
		sq.UnLock()
		return ""
	}
	element := sq.Data[idx]

	sq.UnLock()
	return element
}

func StringSliceHasValue(data []string, val string, asSuffix ...bool) (bool, int) {
	idx := -1
	if asSuffix != nil && len(asSuffix) > 0 && asSuffix[0] {
		valLen := len(val)
		idx = slices.IndexFunc(data, func(s string) bool { return len(s) >= valLen && s[0:valLen] == val })
	} else {
		idx = slices.IndexFunc(data, func(s string) bool { return s == val })
	}
	return idx >= 0, idx
}
