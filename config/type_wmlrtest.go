package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"wisemed-labreaders/sqlitewrapper"
)

type WMLRTest struct {
	Id                          string                   `json:"parametru_echipament_id" bson:"parametru_echipament_id"`
	Code                        string                   `json:"pe_codificare" bson:"pe_codificare"`
	Tag                         string                   `json:"pe_tag" bson:"pe_tag"`
	Active                      string                   `json:"pe_activ" bson:"pe_activ"`
	DecimalsNo                  IntString                `json:"pe_formatare,integer" bson:"pe_formatare"`
	ReagentsSet                 string                   `json:"pe_set_reactivi_buletin" bson:"pe_set_reactivi_buletin"`
	MeasuringUnit               string                   `json:"pe_um" bson:"pe_um"`
	MeasuringUnitWeighting      string                   `json:"pe_ponderare_um" bson:"pe_ponderare_um"`
	NoSynchronisationsOnWisemed IntString                `json:"pe_nr_sincronizari,integer" bson:"pe_nr_sincronizari"`
	Transformations             []WMLRTestTransformation `json:"transformari" bson:"transformari"`
}

func (ai *WMLRTest) ParseFromKnownTest(knownTest sqlitewrapper.SQLKnownTest) {
	ai.Id = strconv.Itoa(knownTest.Id)
	ai.Code = knownTest.Code
	ai.Tag = knownTest.Tag
	if knownTest.Active == 1 {
		ai.Active = "1"
	} else {
		ai.Active = "0"
	}

	switch knownTest.ResultFormatting {
	case sqlitewrapper.KAFormatingToNumberNoDecimals:
		ai.DecimalsNo = IntString("0")
	case sqlitewrapper.KAFormatingToNumber1Decimals:
		ai.DecimalsNo = IntString("1")
	case sqlitewrapper.KAFormatingToNumber2Decimals:
		ai.DecimalsNo = IntString("2")
	case sqlitewrapper.KAFormatingToNumber3Decimals:
		ai.DecimalsNo = IntString("3")
	case sqlitewrapper.KAFormatingToNumber4Decimals:
		ai.DecimalsNo = IntString("4")
	}
	ai.ReagentsSet = knownTest.ResultReagentsSet
	ai.MeasuringUnit = knownTest.ResultMeasureUnit
	ai.MeasuringUnitWeighting = fmt.Sprintf("%f", knownTest.ResultWeighting)
	ai.NoSynchronisationsOnWisemed = IntString("0")
	ai.Transformations = make([]WMLRTestTransformation, len(knownTest.ResultTransformation))

	for i := 0; i < len(knownTest.ResultTransformation); i++ {
		ai.Transformations[i].ParseFromKnownTestTransformation(knownTest.ResultTransformation[i])
	}
}

func (ai *WMLRTest) ParseFromMap(mapData map[string]interface{}) {
	ai.Id = fmt.Sprint(mapData["parametru_echipament_id"])
	ai.Code = fmt.Sprint(mapData["pe_codificare"])
	ai.Tag = fmt.Sprint(mapData["pe_tag"])
	if mapData["pe_activ"].(string) == "1" {
		ai.Active = "1"
	} else {
		ai.Active = "0"
	}

	ai.DecimalsNo = IntString(fmt.Sprint(mapData["pe_formatare"]))
	ai.ReagentsSet = fmt.Sprint(mapData["pe_set_reactivi_buletin"])
	ai.MeasuringUnit = fmt.Sprint(mapData["pe_um"])
	ai.MeasuringUnitWeighting = fmt.Sprint(mapData["pe_ponderare_um"])
	ai.NoSynchronisationsOnWisemed = IntString(fmt.Sprint(mapData["pe_nr_sincronizari"]))
	transformations, _ := mapData["transformari"].([]interface{})

	ai.Transformations = make([]WMLRTestTransformation, len(transformations))
	//now parse transformations
	for i := 0; i < len(transformations); i++ {
		trans := transformations[i].(map[string]interface{})
		ai.Transformations[i].ParseFromMap(trans)
	}
}
func (ai *WMLRTest) ParseFromJSON(jsonData []byte) {
	mapData := make(map[string]interface{})
	json.Unmarshal(jsonData, &mapData)
	ai.ParseFromMap(mapData)
}
func (ai *WMLRTest) ToJSON() []byte {
	analyzerJSON, err := json.Marshal(ai)
	if err != nil {
		return nil
	}
	return analyzerJSON
}
func (ai *WMLRTest) ToStringMap() map[string]string {
	transformationStrMap := make(map[string]string)
	json.Unmarshal(ai.ToJSON(), &transformationStrMap)

	return transformationStrMap
}
