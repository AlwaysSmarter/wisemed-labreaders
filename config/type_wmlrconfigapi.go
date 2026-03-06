package config

import "encoding/json"

type WMLRAPIConfigServer struct {
	APPSerialNo     string   `json:"-" bson:"-"`
	APPAnalyzerName string   `json:"-" bson:"-"`
	APPAnalyzerType WMLRType `json:"-" bson:"-"`
	APPAPIKey       string   `json:"api_key_echipament" bson:"api_key_echipament"`

	Port        string `json:"cfg_reader_api_port" bson:"cfg_reader_api_port"`
	Address     string `json:"cfg_reader_api_address" bson:"cfg_reader_api_address"`
	HTTPSCert   string `json:"cfg_reader_api_cert" bson:"cfg_reader_api_cert"`
	HTTPSKey    string `json:"cfg_reader_api_cert_privatekey" bson:"cfg_reader_api_cert_privatekey"`
	UploadsPath string `json:"cfg_reader_api_uploads_dir" bson:"cfg_reader_api_uploads_dir"`
	DownloadDir string `json:"cfg_reader_api_downloads_dir" bson:"cfg_reader_api_downloads_dir"`

	//WiseMED config
	WMAPIIP       string `json:"cfg_wisemed_ip" bson:"cfg_wisemed_ip"`
	WMAPIPort     string `json:"cfg_wisemed_port" bson:"cfg_wisemed_port"`
	WMAPIProtocol string `json:"cfg_wisemed_protocol" bson:"cfg_wisemed_protocol"`
	WMAPIPath     string `json:"cfg_wisemed_path" bson:"cfg_wisemed_path"`
	WMAPIKey      string `json:"cfg_wisemed_key" bson:"cfg_wisemed_key"`

	//Comm config
	CommType           string `json:"cfg_comm_type" bson:"cfg_comm_type"`
	CommSerialPort     string `json:"cfg_comm_serial_port" bson:"cfg_comm_serial_port"`
	CommSerialBaud     string `json:"cfg_comm_serial_baud" bson:"cfg_comm_serial_baud"`
	CommSerialParity   string `json:"cfg_comm_serial_parity" bson:"cfg_comm_serial_parity"`
	CommSerialStopBits string `json:"cfg_comm_serial_stop_bits" bson:"cfg_comm_serial_stop_bits"`

	CommTCPIPType    string `json:"cfg_comm_tcpip_type" bson:"cfg_comm_tcpip_type"`
	CommTCPIPAddress string `json:"cfg_comm_tcpip_addr" bson:"cfg_comm_tcpip_addr"`
	CommTCPIPPort    string `json:"cfg_comm_tcpip_port" bson:"cfg_comm_tcpip_port"`

	//Analyzer related
	WMLREquipmentId           IntString `json:"echipament_id" bson:"echipament_id"`
	WMLREquipmentCode         string    `json:"cod_echipament" bson:"cod_echipament"`
	WMLREquipmentManufacturer string    `json:"producator_echipament" bson:"producator_echipament"`
	WMLREquipmentSerialNo     string    `json:"numar_serial_echipament" bson:"numar_serial_echipament"`
	WMLRRacksNo               string    `json:"nr_rackuri" bson:"nr_rackuri"`
	WMLRPositionsPerRacks     string    `json:"pozitii_pe_rack" bson:"pozitii_pe_rack"`
	WMLRNameOnReport          string    `json:"nume_pe_raport_final" bson:"nume_pe_raport_final"`
	WMLRMedicalUnitId         string    `json:"unitate_medicala_id" bson:"unitate_medicala_id"`
	WMLREquipmentType         string    `json:"tip_de_echipament_id" bson:"tip_de_echipament_id"`
	WMLRWebSockIp             string    `json:"ip" bson:"ip"`
	WMLRWebSockPort           string    `json:"port" bson:"port"`
	OtherConfig               map[string]string
}

func (srvCfg *WMLRAPIConfigServer) ParseFromWMLRAnalizerInfo(ai WMLRAnalyzerInfo) {
	srvCfg.APPSerialNo = APPSerialNo
	srvCfg.APPAnalyzerName = APPAnalyzerName
	srvCfg.APPAPIKey = APPAPIKey
	srvCfg.APPAnalyzerType = APPAnalyzerType

	srvCfg.WMLREquipmentId = IntString(ai.WMEquipmentId)
	srvCfg.WMLREquipmentCode = ai.Code
	srvCfg.WMLREquipmentManufacturer = ai.Manufacturer
	srvCfg.WMLREquipmentSerialNo = ai.SerialNo
	srvCfg.WMLRRacksNo = ai.RacksNo
	srvCfg.WMLRPositionsPerRacks = ai.PositionsPerRack
	srvCfg.WMLRNameOnReport = ai.NameOnReport
	srvCfg.WMLRMedicalUnitId = ai.WMMedicalUnitId
	srvCfg.WMLREquipmentType = ai.WMEquipmentTypeId
	srvCfg.WMLRWebSockIp = ai.Ip
	srvCfg.WMLRWebSockPort = ai.Port
}
func (srvCfg *WMLRAPIConfigServer) ParseFromMap(mapData map[string]string) {
	srvCfg.WMLREquipmentId = IntString(mapData["echipament_id"])
	srvCfg.WMLREquipmentCode = mapData["cod_echipament"]
	srvCfg.WMLREquipmentManufacturer = mapData["producator_echipament"]
	srvCfg.WMLREquipmentSerialNo = mapData["numar_serial_echipament"]
	srvCfg.WMLRWebSockIp = mapData["ip"]
	srvCfg.WMLRWebSockPort = mapData["port"]
	srvCfg.WMLRRacksNo = mapData["nr_rackuri"]
	srvCfg.WMLRPositionsPerRacks = mapData["pozitii_pe_rack"]
	srvCfg.WMLRNameOnReport = mapData["nume_pe_raport_final"]
	srvCfg.WMLRMedicalUnitId = mapData["unitate_medicala_id"]
	srvCfg.WMLREquipmentType = mapData["tip_de_echipament_id"]

}

func (srvCfg *WMLRAPIConfigServer) ParseFromFullMap(mapData map[string]string) {
	srvCfg.ParseFromMap(mapData)

	srvCfg.APPAPIKey = mapData["api_key_echipament"]
	srvCfg.Port = mapData["cfg_reader_api_port"]
	srvCfg.Address = mapData["cfg_reader_api_address"]
	srvCfg.HTTPSCert = mapData["cfg_reader_api_cert"]
	srvCfg.HTTPSKey = mapData["cfg_reader_api_cert_privatekey"]
	srvCfg.UploadsPath = mapData["cfg_reader_api_uploads_dir"]
	srvCfg.DownloadDir = mapData["cfg_reader_api_downloads_dir"]
	srvCfg.WMAPIIP = mapData["cfg_wisemed_ip"]
	srvCfg.WMAPIPort = mapData["cfg_wisemed_port"]
	srvCfg.WMAPIProtocol = mapData["cfg_wisemed_protocol"]
	srvCfg.WMAPIPath = mapData["cfg_wisemed_path"]
	srvCfg.WMAPIKey = mapData["cfg_wisemed_key"]
	srvCfg.CommType = mapData["cfg_comm_type"]
	srvCfg.CommSerialPort = mapData["cfg_comm_serial_port"]
	srvCfg.CommSerialBaud = mapData["cfg_comm_serial_baud"]
	srvCfg.CommSerialParity = mapData["cfg_comm_serial_parity"]
	srvCfg.CommSerialStopBits = mapData["cfg_comm_serial_stop_bits"]
	srvCfg.CommTCPIPType = mapData["cfg_comm_tcpip_type"]
	srvCfg.CommTCPIPAddress = mapData["cfg_comm_tcpip_addr"]
	srvCfg.CommTCPIPPort = mapData["cfg_comm_tcpip_port"]

}
func (srvCfg *WMLRAPIConfigServer) ParseFromJSON(jsonData []byte) {
	mapData := make(map[string]string)
	json.Unmarshal(jsonData, mapData)
	srvCfg.ParseFromMap(mapData)
}
func (srvCfg *WMLRAPIConfigServer) ToJSON() []byte {
	srvCfgJSON, err := json.Marshal(srvCfg)
	if err != nil {
		return nil
	}
	return srvCfgJSON
}
func (srvCfg *WMLRAPIConfigServer) ToStringMap() map[string]string {
	srvCfgStrMap := make(map[string]string)
	json.Unmarshal(srvCfg.ToJSON(), &srvCfgStrMap)

	srvCfgStrMap["echipament_id"] = string(srvCfg.WMLREquipmentId)

	return srvCfgStrMap
}
