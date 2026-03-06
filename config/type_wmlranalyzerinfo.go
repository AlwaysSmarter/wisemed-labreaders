package config

import (
	"encoding/json"
	"fmt"
)

type WMLRAnalyzerInfo struct {
	Code              string     `json:"cod_echipament" bson:"cod_echipament"`
	Name              string     `json:"nume_echipament" bson:"nume_echipament"`
	APIKey            string     `json:"api_key_echipament" bson:"api_key_echipament"`
	Manufacturer      string     `json:"producator_echipament" bson:"producator_echipament"`
	Type              WMLRType   `json:"tip_analizor" bson:"tip_analizor"`
	SerialNo          string     `json:"numar_serial_echipament" bson:"numar_serial_echipament"`
	Ip                string     `json:"ip" bson:"ip"`
	Port              string     `json:"port" bson:"port"`
	Online            bool       `json:"online" bson:"online"`
	RacksNo           string     `json:"nr_rackuri" bson:"nr_rackuri"`
	PositionsPerRack  string     `json:"pozitii_pe_rack" bson:"pozitii_pe_rack"`
	NameOnReport      string     `json:"nume_pe_raport_final" bson:"nume_pe_raport_final"`
	WMEquipmentId     IntString  `json:"echipament_id,integer" bson:"echipament_id"`
	WMMedicalUnitId   string     `json:"unitate_medicala_id" bson:"unitate_medicala_id"`
	WMEquipmentTypeId string     `json:"tip_de_echipament_id" bson:"tip_de_echipament_id"`
	Tests             []WMLRTest `json:"analize" bson:"analize"`
}

func (ai *WMLRAnalyzerInfo) ParseFromWMLRAPIConfigServer(srvCfg WMLRAPIConfigServer, includeTests bool) {
	ai.Name = APPAnalyzerName
	ai.Type = APPAnalyzerType
	ai.APIKey = APPAPIKey

	ai.Code = srvCfg.WMLREquipmentCode
	ai.Manufacturer = srvCfg.WMLREquipmentManufacturer
	ai.SerialNo = srvCfg.WMLREquipmentSerialNo
	ai.Ip = srvCfg.WMLRWebSockIp
	ai.Port = srvCfg.WMLRWebSockPort
	ai.Online = true
	ai.RacksNo = srvCfg.WMLRRacksNo
	ai.PositionsPerRack = srvCfg.WMLRPositionsPerRacks
	ai.NameOnReport = srvCfg.WMLRNameOnReport
	ai.WMEquipmentId = IntString(srvCfg.WMLREquipmentId)
	ai.WMMedicalUnitId = srvCfg.WMLRMedicalUnitId
	ai.WMEquipmentTypeId = srvCfg.WMLREquipmentType
}
func (ai *WMLRAnalyzerInfo) ParseFromMap(mapData map[string]interface{}) {
	ai.Name = fmt.Sprint(mapData["nume_echipament"])
	ai.APIKey = fmt.Sprint(mapData["api_key_echipament"])
	ai.Manufacturer = fmt.Sprint(mapData["producator_echipament"])
	ai.Code = fmt.Sprint(mapData["cod_echipament"])
	ai.Type = WMLRType(ReturnIntOrZero(fmt.Sprint(mapData["tip_analizor"])))
	ai.SerialNo = fmt.Sprint(mapData["numar_serial_echipament"])
	ai.Ip = fmt.Sprint(mapData["ip"])
	ai.Port = fmt.Sprint(mapData["port"])
	ai.Online = false
	if mapData["online"] == "1" || mapData["online"] == "on" || mapData["online"] == "true" {
		ai.Online = true
	}
	ai.RacksNo = fmt.Sprint(mapData["nr_rackuri"])
	ai.PositionsPerRack = fmt.Sprint(mapData["pozitii_pe_rack"])
	ai.NameOnReport = fmt.Sprint(mapData["nume_pe_raport_final"])
	ai.WMEquipmentId = IntString(fmt.Sprint(mapData["echipament_id"]))
	ai.WMMedicalUnitId = fmt.Sprint(mapData["unitate_medicala_id"])
	ai.WMEquipmentTypeId = fmt.Sprint(mapData["tip_de_echipament_id"])

	tests, _ := mapData["lran_transformations"].([]interface{})

	ai.Tests = make([]WMLRTest, len(tests))
	//now parse transformations
	for i := 0; i < len(tests); i++ {
		an := tests[i].(map[string]interface{})
		ai.Tests[i].ParseFromMap(an)
	}
}
func (ai *WMLRAnalyzerInfo) ParseFromJSON(jsonData []byte) {
	mapData := make(map[string]interface{})
	json.Unmarshal(jsonData, &mapData)
	ai.ParseFromMap(mapData)
}
func (ai *WMLRAnalyzerInfo) ToJSON() []byte {
	analyzerJSON, err := json.Marshal(ai)
	if err != nil {
		return nil
	}
	return analyzerJSON
}
func (ai *WMLRAnalyzerInfo) ToStringMap() map[string]string {
	analyzerStrMap := make(map[string]string)
	json.Unmarshal(ai.ToJSON(), &analyzerStrMap)
	analyzerStrMap["online"] = "false"
	if ai.Online {
		analyzerStrMap["online"] = "true"
	}
	analyzerStrMap["echipament_id"] = string(ai.WMEquipmentId)

	return analyzerStrMap
}
func (ai *WMLRAnalyzerInfo) ToInterfaceMap() map[string]interface{} {
	analyzerStrMap := make(map[string]interface{})
	json.Unmarshal(ai.ToJSON(), &analyzerStrMap)
	analyzerStrMap["online"] = "false"
	if ai.Online {
		analyzerStrMap["online"] = "true"
	}
	analyzerStrMap["echipament_id"] = string(ai.WMEquipmentId)

	return analyzerStrMap
}
