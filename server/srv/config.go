package srv

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"wisemed-labreaders/comm"
	"wisemed-labreaders/config"
	"wisemed-labreaders/sqlitewrapper"
	"wisemed-labreaders/wisemed"
)

func (s *SIUIAPIServerType) APIConfigSave(response http.ResponseWriter, request *http.Request, saveData map[string]string) (bool, error) {
	var err error = nil
	_, ok := saveData["SaveCfg"]
	if ok {
		commParamsModified, err := updateWiseMEDConfigParams(saveData)
		if err != nil {
			return false, err
		}
		commParamsModifiedFromNotifier, err := s.APINotifier(response, request, true)
		if err != nil {
			return false, err
		}
		if commParamsModified || commParamsModifiedFromNotifier {
			comm.ReStartEquipmentCommunication(s.CommHandlerCreator)
		}
	}
	return ok, err
}
func (s *SIUIAPIServerType) APIHowTOConfig(response http.ResponseWriter, request *http.Request) {
	tplData := parseParameters(s, request, true)
	serveHTMLTpl(fmt.Sprintf("how-to%sindex.tpl.html", string(os.PathSeparator)), response, tplData, true)
}

func (s *SIUIAPIServerType) APIConfig(response http.ResponseWriter, request *http.Request) {

	tplDataNoConfig := parseParameters(s, request, false)

	_, err := s.APIConfigSave(response, request, tplDataNoConfig)
	if err != nil {
		RegisterAppHtmlTplError(err.Error(), tplDataNoConfig)
	}
	includeDBConfigInTplData(tplDataNoConfig)

	hideEquipmentForm := false
	if config.ServerConfiguration.WMAPIIP == "" ||
		config.ServerConfiguration.WMAPIKey == "" ||
		config.ServerConfiguration.WMAPIPort == "" ||
		config.ServerConfiguration.WMAPIProtocol == "" ||
		config.ServerConfiguration.WMAPIPath == "" {
		hideEquipmentForm = true
	}
	if s.OtherConfig != nil {
		tplDataNoConfig["OTHER_CONFIG"] = s.OtherConfig(tplDataNoConfig)
	}
	tplDataNoConfig["CONFIG_FORM"] = returnConfigFormTpl(tplDataNoConfig, hideEquipmentForm)
	serveHTMLTpl("config.tpl.html", response, tplDataNoConfig, true)

}
func (s *SIUIAPIServerType) APIWMConfig(response http.ResponseWriter, request *http.Request) {

	tplDataNoConfig := parseParameters(s, request, false)

	_, err := s.APIConfigSave(response, request, tplDataNoConfig)
	if err != nil {
		RegisterAppHtmlTplError(err.Error(), tplDataNoConfig)
	}
	includeDBConfigInTplData(tplDataNoConfig)

	serveHTMLTpl("login.tpl.html", response, tplDataNoConfig, true)

}
func (s *SIUIAPIServerType) APICAPIWMConfigonfig(response http.ResponseWriter, request *http.Request) {

	tplDataNoConfig := parseParameters(s, request, false)

	_, err := s.APIConfigSave(response, request, tplDataNoConfig)
	if err != nil {
		RegisterAppHtmlTplError(err.Error(), tplDataNoConfig)
	}
	includeDBConfigInTplData(tplDataNoConfig)

	hideEquipmentForm := false
	if config.ServerConfiguration.WMAPIIP == "" ||
		config.ServerConfiguration.WMAPIKey == "" ||
		config.ServerConfiguration.WMAPIPort == "" ||
		config.ServerConfiguration.WMAPIProtocol == "" ||
		config.ServerConfiguration.WMAPIPath == "" {
		hideEquipmentForm = true
	}
	tplDataNoConfig["CONFIG_FORM"] = returnConfigFormTpl(tplDataNoConfig, hideEquipmentForm)
	serveHTMLTpl("config.tpl.html", response, tplDataNoConfig, true)

}

func updateWiseMEDConfigParams(cfg map[string]string) (bool, error) {
	commParamsModified := false
	cfgJson, err := json.Marshal(config.ServerConfiguration)
	if err != nil {
		return commParamsModified, err
	}
	cfgMap := make(map[string]string)
	json.Unmarshal(cfgJson, &cfgMap)

	//Heew we will check if the LIS communication parameters have changed
	for key, val := range cfg {
		currVal, ok := cfgMap[key]
		if ok && currVal != val {
			fmt.Printf("Setting configuration pair %s = %s\n", key, val)
			err := sqlitewrapper.SetConfigurationPair(key, val)
			if err != nil {
				fmt.Printf("Setting configuration pair %s = %s ERROR:\n", key, val, err)
				return commParamsModified, err
			}
			if comm.IetLISCommunicationParam(key) {
				commParamsModified = true
			}
		}
		if !ok && strings.HasPrefix(key, "othercfg_") {
			fmt.Printf("Setting other configuration pair %s = %s\n", key, val)
			err := sqlitewrapper.SetConfigurationPair(key, val)
			if err != nil {
				fmt.Printf("Setting other configuration pair %s = %s ERROR:\n", key, val, err)
				return commParamsModified, err
			}
		}
	}
	config.ReadConfig()
	return commParamsModified, nil
}

func returnConfigFormTpl(tplData map[string]string, hideEquipmentForm bool) string {
	if hideEquipmentForm {
		tplData["EQUIPMENT_FORM_OPTIONAL"] = "optional-form"
		tplData["EQUIPMENT_FORM_HIDDEN"] = "hide"
	} else {
		medicalUnits, err := wisemed.WMAPIGetMedicalUnits()
		if err != nil {
			RegisterAppHtmlTplError(err.Error(), tplData)
		}
		tplData["MEDICAL_UNITS_JSON"] = medicalUnits

		wmAnalyzerTypes, err := wisemed.WMAPIGetWMAnalyzerTypes()
		if err != nil {
			RegisterAppHtmlTplError(err.Error(), tplData)
		}
		tplData["WM_ANALIZER_TYPES_JSON"] = wmAnalyzerTypes
	}

	cfgStr, err := returnParsedHTMLTplFile("config_form.tpl.html", tplData, true)
	if err != nil {
		RegisterAppHtmlTplError(err.Error(), tplData)
	}

	return string(cfgStr)
}

func (s *SIUIAPIServerType) APIAPIGetConfig(response http.ResponseWriter, request *http.Request) {
	//tplData := parseParameters(s, request, true)
	analyzer := config.WMLRAnalyzerInfo{}
	analyzer.ParseFromWMLRAPIConfigServer(config.ServerConfiguration, true)

	serveJSON(nil, response, analyzer.ToStringMap())
}
