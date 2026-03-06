package general

import "encoding/json"
import "fmt"

type WMLRLoginRequest struct {
	Username      string `json:"username" bson:"username"`
	Password      string `json:"password" bson:"password"`
	MedicalUnitId string `json:"medical_unit_id" bson:"medical_unit_id"`
	DeviceId      string `json:"device_id" bson:"device_id"`
	DeviceName    string `json:"device_name" bson:"device_name"`
}
type WMLRLoginInfo struct {
	UserId             json.Number `json:"user_id" bson:"user_id"`
	Login              string      `json:"login" bson:"login"`
	FirstName          string      `json:"first_name" bson:"first_name"`
	LastName           string      `json:"last_name" bson:"last_name"`
	UserType           string      `json:"user_type" bson:"user_type"`
	UserEmail          string      `json:"user_email" bson:"user_email"`
	LoginToken         string      `json:"login_token" bson:"login_token"`
	MobilePrefix       string      `json:"user_mobile_country_prefix" bson:"user_mobile_country_prefix"`
	MobileNumber       string      `json:"user_mobile_number" bson:"user_mobile_number"`
	GlobalCacheVersion string      `json:"global_cache_version" bson:"global_cache_version"`
}

func (lr *WMLRLoginRequest) ParseFromMap(mapData map[string]interface{}) {
	lr.Username = fmt.Sprint(mapData["username"])
	lr.Password = fmt.Sprint(mapData["password"])
	lr.MedicalUnitId = fmt.Sprint(mapData["medical_unit_id"])
	lr.DeviceId = fmt.Sprint(mapData["device_id"])
	lr.DeviceName = fmt.Sprint(mapData["device_name"])
}
func (lr *WMLRLoginRequest) ParseFromJSON(jsonData []byte) {
	mapData := make(map[string]interface{})
	json.Unmarshal(jsonData, &mapData)
	lr.ParseFromMap(mapData)
}
func (lr *WMLRLoginRequest) ToJSON() []byte {
	analyzerJSON, err := json.Marshal(lr)
	if err != nil {
		return nil
	}
	return analyzerJSON
}
func (lr *WMLRLoginRequest) ToStringMap() map[string]string {
	loginReqMap := make(map[string]string)
	json.Unmarshal(lr.ToJSON(), &loginReqMap)
	loginReqMap["username"] = string(lr.Username)
	loginReqMap["password"] = string(lr.Password)
	loginReqMap["medical_unit_id"] = string(lr.MedicalUnitId)
	loginReqMap["device_id"] = string(lr.DeviceId)
	loginReqMap["device_name"] = string(lr.DeviceName)

	return loginReqMap
}
func (lr *WMLRLoginRequest) ToInterfaceMap() map[string]interface{} {
	loginReqMap := make(map[string]interface{})
	json.Unmarshal(lr.ToJSON(), &loginReqMap)
	loginReqMap["username"] = string(lr.Username)
	loginReqMap["password"] = string(lr.Password)
	loginReqMap["medical_unit_id"] = string(lr.MedicalUnitId)
	loginReqMap["device_id"] = string(lr.DeviceId)
	loginReqMap["device_name"] = string(lr.DeviceName)

	return loginReqMap
}

func (li *WMLRLoginInfo) ParseFromMap(mapData map[string]interface{}) {
	li.UserId = json.Number(fmt.Sprint(mapData["user_id"]))
	li.Login = fmt.Sprint(mapData["login"])
	li.FirstName = fmt.Sprint(mapData["first_name"])
	li.LastName = fmt.Sprint(mapData["last_name"])
	li.UserType = fmt.Sprint(mapData["user_type"])
	li.UserEmail = fmt.Sprint(mapData["user_email"])
	li.LoginToken = fmt.Sprint(mapData["login_token"])
	li.MobilePrefix = fmt.Sprint(mapData["user_mobile_country_prefix"])
	li.MobileNumber = fmt.Sprint(mapData["user_mobile_number"])
	li.GlobalCacheVersion = fmt.Sprint(mapData["global_cache_version"])
}
func (li *WMLRLoginInfo) ParseFromStrMap(mapData map[string]string) {
	li.UserId = json.Number(mapData["user_id"])
	li.Login = mapData["login"]
	li.FirstName = mapData["first_name"]
	li.LastName = mapData["last_name"]
	li.UserType = mapData["user_type"]
	li.UserEmail = mapData["user_email"]
	li.LoginToken = mapData["login_token"]
	li.MobilePrefix = mapData["user_mobile_country_prefix"]
	li.MobileNumber = mapData["user_mobile_number"]
	li.GlobalCacheVersion = mapData["global_cache_version"]
}
func (lr *WMLRLoginInfo) ParseFromJSON(jsonData []byte) {
	mapData := make(map[string]interface{})
	json.Unmarshal(jsonData, &mapData)
	lr.ParseFromMap(mapData)
}
func (lr *WMLRLoginInfo) ToJSON() []byte {
	analyzerJSON, err := json.Marshal(lr)
	if err != nil {
		return nil
	}
	return analyzerJSON
}

func (li *WMLRLoginInfo) ToStringMap() map[string]string {
	loginReqMap := make(map[string]string)
	json.Unmarshal(li.ToJSON(), &loginReqMap)
	loginReqMap["user_id"] = string(li.UserId)
	loginReqMap["login"] = string(li.Login)
	loginReqMap["first_name"] = string(li.FirstName)
	loginReqMap["last_name"] = string(li.LastName)
	loginReqMap["user_type"] = string(li.UserType)
	loginReqMap["user_email"] = string(li.UserEmail)
	loginReqMap["login_token"] = string(li.LoginToken)
	loginReqMap["user_mobile_country_prefix"] = string(li.MobilePrefix)
	loginReqMap["user_mobile_number"] = string(li.MobileNumber)
	loginReqMap["global_cache_version"] = string(li.GlobalCacheVersion)

	return loginReqMap
}

func (lr *WMLRLoginInfo) ToInterfaceMap() map[string]interface{} {
	loginReqMap := make(map[string]interface{})
	json.Unmarshal(lr.ToJSON(), &loginReqMap)
	loginReqMap["user_id"] = string(lr.UserId)
	loginReqMap["login"] = string(lr.Login)
	loginReqMap["first_name"] = string(lr.FirstName)
	loginReqMap["last_name"] = string(lr.LastName)
	loginReqMap["user_type"] = string(lr.UserType)
	loginReqMap["user_email"] = string(lr.UserEmail)
	loginReqMap["login_token"] = string(lr.LoginToken)
	loginReqMap["user_mobile_country_prefix"] = string(lr.MobilePrefix)
	loginReqMap["user_mobile_number"] = string(lr.MobileNumber)
	loginReqMap["global_cache_version"] = string(lr.GlobalCacheVersion)

	return loginReqMap
}
