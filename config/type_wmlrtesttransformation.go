package config

import (
	"encoding/json"
	"fmt"
	"wisemed-labreaders/sqlitewrapper"
)

type WMLRTestTransformation struct {
	From string `json:"from_str" bson:"from_str"`
	To   string `json:"to_str" bson:"to_str"`
}

func (ai *WMLRTestTransformation) ParseFromKnownTestTransformation(knownTestTrans sqlitewrapper.SQLKnownTestTrans) {
	ai.From = knownTestTrans.From
	ai.To = knownTestTrans.To
}
func (ai *WMLRTestTransformation) ParseFromMap(mapData map[string]interface{}) {
	ai.From = fmt.Sprint(mapData["from_str"])
	ai.To = fmt.Sprint(mapData["to_str"])
}
func (ai *WMLRTestTransformation) ParseFromJSON(jsonData []byte) {
	mapData := make(map[string]interface{})
	json.Unmarshal(jsonData, &mapData)
	ai.ParseFromMap(mapData)
}
func (ai *WMLRTestTransformation) ToJSON() []byte {
	analyzerJSON, err := json.Marshal(ai)
	if err != nil {
		return nil
	}
	return analyzerJSON
}
func (ai *WMLRTestTransformation) ToStringMap() map[string]string {
	transformationStrMap := make(map[string]string)
	json.Unmarshal(ai.ToJSON(), &transformationStrMap)

	return transformationStrMap
}
