package wisemed

import (
	"encoding/json"
	"errors"
	"fmt"
	"wisemed-labreaders/config"
	"wisemed-labreaders/general"
	"wisemed-labreaders/sqlitewrapper"
)

func WMAPILogin(loginData general.WMLRLoginRequest) (*general.WMLRLoginInfo, error) {
	//try to save to WiseMED
	_, err := isWiseMEDAPIConfigOK()
	if err != nil {
		return nil, err
	}

	loginInfo := general.WMLRLoginInfo{}
	//analyzer.ParseFromWMLRAPIConfigServer(config.ServerConfiguration, true)

	wmAnalyzerData, err := wiseMEDAPIPutByteArr("/administrative/login", loginData.ToJSON())
	if err != nil {
		return nil, err
	}
	// Convert response body to out Analyzer structure
	json.Unmarshal(wmAnalyzerData, &loginInfo)

	return &loginInfo, nil
}
func WMAPIReaderInitialization(includeOnlineStatusTests bool) (*config.WMLRAnalyzerInfo, error) {
	//try to save to WiseMED
	_, err := isWiseMEDAPIConfigOK()
	if err != nil {
		return nil, err
	}

	analyzer := config.WMLRAnalyzerInfo{}

	analyzer.ParseFromWMLRAPIConfigServer(config.ServerConfiguration, true)

	if includeOnlineStatusTests {
		kt, err := sqlitewrapper.GetKnownTests()
		if err != nil {
			return nil, err
		}

		for _, test := range kt {
			tmpTest := config.WMLRTest{}
			tmpTest.ParseFromKnownTest(test)
			analyzer.Tests = append(analyzer.Tests, tmpTest)
		}
	}

	returnedAnalyzer := map[string]interface{}{}
	wmAnalyzerData, err := wiseMEDAPIPutByteArr("/administrative/analyzer", analyzer.ToJSON())
	err = json.Unmarshal(wmAnalyzerData, &returnedAnalyzer)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Returned analyzer data is not known.\n%s\n%s", string(wmAnalyzerData), err.Error()))
	}
	fmt.Println(string(analyzer.ToJSON()))
	if err != nil {
		return nil, err
	}
	// Convert response body to out Analyzer structure
	json.Unmarshal(wmAnalyzerData, &analyzer)

	return &analyzer, nil
}

func WMAPIConfirmFileServiceResults(fsr []config.WMLRFileServiceResult) ([]byte, error) {
	//try to save to WiseMED
	_, err := isWiseMEDAPIConfigOK()
	if err != nil {
		return nil, err
	}

	fsrJSON, err := json.Marshal(fsr)
	if err != nil {
		return nil, err
	}

	//fmt.Printf("PUT DATA: %s", fsrJSON)
	wmUpdatedData, err := wiseMEDAPIPutByteArr("/fileforanalyzer/results/", fsrJSON)
	if err != nil {
		return nil, err
	}

	return wmUpdatedData, nil
}
func WMAPIAnalyzerFileData(fileId int) (*sqlitewrapper.SQLOrder, error) {
	//try to save to WiseMED
	_, err := isWiseMEDAPIConfigOK()
	if err != nil {
		return nil, err
	}

	wmAnalyzerData, err := wiseMEDAPIGet(fmt.Sprintf("/fileforanalyzer/%d/%d/", fileId, config.ReturnIntOrZero(string(config.ServerConfiguration.WMLREquipmentId))), nil)
	if err != nil {
		return nil, err
	}
	// Convert response body to out Analyzer structure
	fileData := sqlitewrapper.SQLOrder{}
	err = json.Unmarshal(wmAnalyzerData, &fileData)
	if err != nil {
		return nil, err
	}
	return &fileData, nil
}

func WMAPIGetMedicalUnits() (string, error) {
	//try to save to WiseMED
	_, err := isWiseMEDAPIConfigOK()
	if err != nil {
		return "", err
	}

	//login := map[string]string{  "username" : "radu", "password": "radup", "medical_unit_id":"1"}
	medicalUnits, err := wiseMEDAPIGet("/administrative/medicalunits", nil)
	if err != nil {
		return "", err
	}
	//// Convert response body to Todo struct
	//responseJSON := make(map[string]string)
	//json.Unmarshal(medicalUnits, &responseJSON)
	//fmt.Printf("API Response as struct:\n%+v\n", responseJSON)
	return string(medicalUnits), nil
}

func WMAPIGetWMAnalyzerTypes() (string, error) {
	//try to save to WiseMED
	_, err := isWiseMEDAPIConfigOK()
	if err != nil {
		return "", err
	}

	//login := map[string]string{  "username" : "radu", "password": "radup", "medical_unit_id":"1"}
	wmAnalyzerTypes, err := wiseMEDAPIGet("/administrative/wmanalyzertypes", nil)
	if err != nil {
		return "", err
	}
	//// Convert response body to Todo struct
	//responseJSON := make(map[string]string)
	//json.Unmarshal(medicalUnits, &responseJSON)
	//fmt.Printf("API Response as struct:\n%+v\n", responseJSON)
	return string(wmAnalyzerTypes), nil
}
