package srv

import (
	"fmt"
	"net/http"
	"wisemed-labreaders/comm"
	"wisemed-labreaders/config"
	"wisemed-labreaders/general"
	"wisemed-labreaders/wisemed"
)

func (s *SIUIAPIServerType) APILogin(response http.ResponseWriter, request *http.Request) {
	tplData := parseParameters(s, request, true)
	if s.LoggedinUser != nil {
		s.APIMenu(response, request)
		return
	}
	//if the user required to save the config
	_, ok := tplData["SaveCfg"]
	if ok {
		if pass, ok := tplData["cfg_wisemed_pass"]; ok {
			if pass != "Admin pass" {
				RegisterAppHtmlTplError("Wrong admin password", tplData)
				serveHTMLTpl("login.tpl.html", response, tplData, true)
				return
			}
		} else {
			RegisterAppHtmlTplError("Wrong admin password", tplData)
			serveHTMLTpl("login.tpl.html", response, tplData, true)
			return
		}

		_, err := s.APIConfigSave(response, request, tplData)
		if err != nil {
			RegisterAppHtmlTplError(err.Error(), tplData)
		}
		includeDBConfigInTplData(tplData)
	}

	//not loggedin yet
	_, ok = tplData["Login"]
	if ok {
		lr := general.WMLRLoginRequest{}

		lr.Username = tplData["APP_login"]
		lr.Password = tplData["APP_pass"]
		lr.MedicalUnitId = ""
		lr.DeviceId = "1"
		lr.DeviceName = "WMLabReader"

		li, err := wisemed.WMAPILogin(lr)
		if err != nil {
			RegisterAppHtmlTplError(err.Error(), tplData)
			tplData["APP_pass"] = ""
		} else {
			s.SetLoggedinUser(li)
			sess := s.SessManager.SessionStart(response, request)
			liStr := li.ToStringMap()
			sess.Set("loggedinuser", s.LoggedinUser.Login)
			sess.Set("loggedinuserdata", liStr)
			tplData["APP_LOGGEDIN_USER"] = fmt.Sprintf("%s %s", s.LoggedinUser.FirstName, s.LoggedinUser.LastName)

			/**
			 * WMR was run before, we already have the ID of the analyzer in WiseMED - now we will inform WiseMED that the analyzer is live,
			 * providing the IP and port in which its WebSocketsAPI and API is running
			 */
			commParamsModified, err := s.APINotifier(response, request, false)
			if err != nil {
				RegisterAppHtmlTplError(fmt.Sprintf("%s%s", "Nu s-a reusit initializarea analizorului\n", err.Error()), tplData)
			} else {
				if commParamsModified {
					comm.ReStartEquipmentCommunication(s.CommHandlerCreator)
				}
				tmpInt := config.ReturnIntOrZero(string(config.ServerConfiguration.WMLREquipmentId))
				if tmpInt <= 0 {
					//Try to initialize if the anlyzer is not yet initialized in WiseMED
					s.APIInitialization(response, request, false)
				} else {
					s.APIMenu(response, request)
				}
				return
			}
		}
	}

	serveHTMLTpl("login.tpl.html", response, tplData, true)
}

func (s *SIUIAPIServerType) APILogout(response http.ResponseWriter, request *http.Request) {
	tplData := parseParameters(s, request, true)
	s.LoggedinUser = nil
	s.Session.Set("loggedinuser", nil)
	serveHTMLTpl("login.tpl.html", response, tplData, true)
	return
}
