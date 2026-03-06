package srv

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"wisemed-labreaders/comm"
	"wisemed-labreaders/sqlitewrapper"
)

func (s *SIUIAPIServerType) APIGetKnownTests(response http.ResponseWriter, request *http.Request) {
	tplDataNoConfig := parseParameters(s, request, false)

	kt, err := sqlitewrapper.GetKnownTests()
	if err != nil {
		RegisterAppHtmlTplError(err.Error(), tplDataNoConfig)
	}

	ktJSON, err := json.Marshal(kt)
	if err != nil {
		RegisterAppHtmlTplError(err.Error(), tplDataNoConfig)
	}

	tplDataNoConfig["KT_JSON"] = string(ktJSON)
	serveHTMLTpl("known_tests.tpl.html", response, tplDataNoConfig, true)

}

func (s *SIUIAPIServerType) APISaveKnownTests(response http.ResponseWriter, request *http.Request) {
	tplDataNoConfig := parseParameters(s, request, false)

	ktID, err := strconv.Atoi(tplDataNoConfig["EditedId"])
	if err != nil {
		ktID = 0
	}

	shouldnformWM := false
	switch tplDataNoConfig["EditAction"] {
	case "save":
		err := sqlitewrapper.SaveKnownTestFromMap(ktID, tplDataNoConfig)
		if err != nil {
			RegisterAppHtmlTplError(err.Error(), tplDataNoConfig)
			serveHTMLTpl("known_test_edit.tpl.html", response, tplDataNoConfig, true)
			return
		}
		shouldnformWM = true
		//try to save it online to WiseMED...
		_, _, err = tryToInitializeOnApi(true)
		if err != nil {
			RegisterAppHtmlTplError("WiseMED tests not saved: "+err.Error(), tplDataNoConfig)
			serveHTMLTpl("known_test_edit.tpl.html", response, tplDataNoConfig, true)
			return
		}

		break
	case "del":
		if ktID <= 0 {
			RegisterAppHtmlTplError("Don't know what test to delete", tplDataNoConfig)
		} else {
			err = sqlitewrapper.DeleteKnownTest(ktID)
			if err != nil {
				RegisterAppHtmlTplError(err.Error(), tplDataNoConfig)
			}
		}
		shouldnformWM = true
		break
	case "delete":
		if ktID <= 0 {
			RegisterAppHtmlTplError("Don't know what test to delete", tplDataNoConfig)
		} else {
			err = sqlitewrapper.DeleteKnownTest(ktID)
			if err != nil {
				RegisterAppHtmlTplError(err.Error(), tplDataNoConfig)
			}
		}
		shouldnformWM = true
		break
	default:
		//edit
		ktObj, err := sqlitewrapper.GetKnownTest(ktID)
		if err != nil {
			RegisterAppHtmlTplError(err.Error(), tplDataNoConfig)
		} else {
			ktMap, err := ktObj.ToMap()
			if err != nil {
				RegisterAppHtmlTplError(err.Error(), tplDataNoConfig)
			} else {
				for key, val := range ktMap {
					tplDataNoConfig[key] = val
				}
				serveHTMLTpl("known_test_edit.tpl.html", response, tplDataNoConfig, true)
				return
			}
		}
		break
	}

	if shouldnformWM {
		_, _, err = tryToInitializeOnApi(true)
		if err != nil {
			RegisterAppHtmlTplError(err.Error(), tplDataNoConfig)
			return
		}
	}

	s.APIGetKnownTests(response, request)
	return
}

func (s *SIUIAPIServerType) APIAPIGetTests(response http.ResponseWriter, request *http.Request) {
	//tplData := parseParameters(s, request, true)

	//tplDataNoConfig := parseParameters(s, request, false)

	kt, err := sqlitewrapper.GetKnownTests()
	if err != nil {
		serveJSON(err, response, nil)
		return
	}
	respMap := make([]map[string]string, 0)
	for i := 0; i < len(kt); i++ {
		tmpMap, err := kt[0].ToMap()
		if err != nil {
			serveJSON(err, response, nil)
			return
		}
		respMap = append(respMap, tmpMap)
	}
	serveJSON(nil, response, respMap)
}

func (s *SIUIAPIServerType) APIReloadKnownTestsFromAnalyzer(response http.ResponseWriter, request *http.Request) {
	tplData := parseParameters(s, request, true)
	sentTo, err := comm.InitiateCommand(s.CommHandlerCreator, "reloadknowntests")
	if err != nil {
		RegisterAppHtmlTplError(err.Error(), tplData)
		serveJSON(err, response, nil)
		return
	}
	if sentTo <= 0 {
		RegisterAppHtmlTplError("Analyzor not connected", tplData)
		serveJSON(err, response, nil)

	}
	serveJSON(nil, response, nil)
	return
}

func (s *SIUIAPIServerType) APIAddMissingDefaultKT(response http.ResponseWriter, request *http.Request) {
	tplData := parseParameters(s, request, true)
	respArr := map[string]interface{}{
		"inserted_no": 0,
	}
	if s.DefaultKTData != nil {
		kt := s.DefaultKTData()

		savedKT := 0
		var errTxt = ""
		for _, tst := range kt {
			name, ok := tst["name"]
			if !ok {
				continue
			}
			code, ok := tst["code"]
			if !ok {
				continue
			}
			um, ok := tst["um"]
			if !ok {
				continue
			}

			ktArr, err := sqlitewrapper.GetKnownTestsQuery(sqlitewrapper.SQLKnownTestQuery{Code: code})
			if err != nil {
				RegisterAppHtmlTplError(err.Error(), tplData)
				serveJSON(err, response, nil)
			}
			var sqlKT sqlitewrapper.SQLKnownTest
			if len(ktArr) > 0 {
				sqlKT = ktArr[0]
			} else {
				sqlKT = sqlitewrapper.SQLKnownTest{
					Id: -1,
				}
			}

			sqlKT.Code = code
			sqlKT.Details = name
			sqlKT.ResultTransformation = []sqlitewrapper.SQLKnownTestTrans{}
			sqlKT.ResultMeasureUnit = um
			if sqlKT.Tag == "" {
				sqlKT.Tag = name
			}

			resType, ok := tst["restype"]
			if !ok {
				resType = ""
			}
			if resType == "2" {
				sqlKT.ResultType = sqlitewrapper.KATypeQalitative
			} else {
				sqlKT.ResultType = sqlitewrapper.KATypeQuantitative
			}

			inact, ok := tst["inactive"]
			if !ok {
				inact = ""
			}
			if inact == "1" {
				sqlKT.Active = 0
			} else {
				sqlKT.Active = 1
			}

			err = sqlitewrapper.SaveKnownTest(sqlKT.Id, sqlKT)
			if err != nil {
				errTxt += err.Error()
			}
			savedKT++
		}
		respArr["inserted_no"] = savedKT

		if errTxt != "" {
			RegisterAppHtmlTplError(errTxt, tplData)
			serveJSON(errors.New(errTxt), response, nil)
		}

		return
	}
	serveJSON(nil, response, respArr)
	return
}
