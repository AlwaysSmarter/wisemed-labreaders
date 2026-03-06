package srv

import (
	"fmt"
	"net/http"
	"wisemed-labreaders/comm"
	"wisemed-labreaders/config"
	"wisemed-labreaders/wisemed"
)

var hideEquipmentForm = false

func (s *SIUIAPIServerType) APIInitialization(response http.ResponseWriter, request *http.Request, onlyWiseMEDConfig bool) bool {
	tplData := parseParameters(s, request, true)

	savedData, err := s.APIConfigSave(response, request, tplData)
	if err != nil {
		RegisterAppHtmlTplError(err.Error(), tplData)
	}
	if savedData {
		shouldReturn := false
		tplData, shouldReturn = tryToInitialize(s, request)
		if shouldReturn {
			//nohting to do anymore - return and serve the requested page
			return true
		}

		if s.LoggedinUser.Login == "" {
			//Show login window after setting the
			s.APILogin(response, request)
			return false
		}
	}
	includeDBConfigInTplData(tplData)

	if config.IsMissingAnyFirstRunInfo() {
		hideEquipmentForm = true
	}

	tplData["CONFIG_FORM"] = returnConfigFormTpl(tplData, hideEquipmentForm)
	serveHTMLTpl("first_run.tpl.html", response, tplData, true)

	return false
}

func tryToInitializeOnApi(includeOnlineStatusTests bool) (map[string]string, bool, error) {
	wmAnData, err := wisemed.WMAPIReaderInitialization(includeOnlineStatusTests)
	if err != nil {
		return nil, false, err
	}
	//save received config to DB and then reload the config to see if the equipment received and ID form WiseMED
	wmAnDataStrMap := wmAnData.ToStringMap()

	commParamsModified, err := updateWiseMEDConfigParams(wmAnDataStrMap)
	if err != nil {
		return nil, false, err
	}
	fmt.Printf("\nAnalyzer data saved %q", wmAnDataStrMap)

	return wmAnDataStrMap, commParamsModified, nil
}
func tryToInitialize(s *SIUIAPIServerType, request *http.Request) (map[string]string, bool) {
	//try to initialize the reader here
	tplData := make(map[string]string)
	//Deleting the save key so it won't try to save again on the next load
	request.Form.Del("SaveCfg")

	if request.Form.Get("EquipmentFormHidden") != "hide" {
		//Equipment form was not hidden, hence we try to save the data
		//try to initialize the equipment in WiseMED and receive and ID for it
		wmAnDataStrMap, commParamsModified, err := tryToInitializeOnApi(false)
		if err != nil {
			RegisterAppHtmlTplError(err.Error(), tplData, request)
			return tplData, true
		}
		if commParamsModified {
			comm.ReStartEquipmentCommunication(s.CommHandlerCreator)
		}

		tplData = wmAnDataStrMap
	}

	tmpInt := config.ReturnIntOrZero(string(config.ServerConfiguration.WMLREquipmentId))
	if tmpInt > 0 {
		return tplData, true
	}

	return tplData, false
}
