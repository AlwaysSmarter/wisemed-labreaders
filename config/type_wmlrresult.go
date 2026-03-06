package config

import "encoding/json"

type WMLRFileServiceResult struct {
	FileId               string `json:"fisa_id" bson:"fisa_id"`
	ServiceTag           string `json:"sm_service_tag" bson:"sm_service_tag"`
	AnalyzerNameOnReport string `json:"flsm_den_analizor" bson:"flsm_den_analizor"`
	ReagentsSet          string `json:"flsm_rescriere_set_reactivi" bson:"flsm_rescriere_set_reactivi"`
	MeasuringUnit        string `json:"flsm_rescriere_um" bson:"flsm_rescriere_um"`
	Result               string `json:"sm_rezultat" bson:"sm_rezultat"`
	Interpretation       string `json:"sm_interpretare" bson:"sm_interpretare"`
	WorkingDatetime      string `json:"sm_lucrata_la_data_ora" bson:"sm_lucrata_la_data_ora"`
	WorkedBy             string `json:"sm_lucrata_de_id" bson:"sm_lucrata_de_id"`
	ConfirmationDatetime string `json:"flsm_confirmare_rez_analizor_la_data_ora" bson:"flsm_confirmare_rez_analizor_la_data_ora"`
	ConfirmedBy          string `json:"flsm_confirmare_rez_analizor_de_id" bson:"flsm_confirmare_rez_analizor_de_id"`
}

func (fsr *WMLRFileServiceResult) ToJSON() []byte {
	fsrJSON, err := json.Marshal(fsr)
	if err != nil {
		return nil
	}
	return fsrJSON
}
