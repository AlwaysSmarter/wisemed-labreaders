package srv

import (
	"fmt"
	"net/http"
	"os"
	"wisemed-labreaders/comm"
)

func (s *SIUIAPIServerType) APICommunication(response http.ResponseWriter, request *http.Request) {
	tplData := parseParameters(s, request, false)
	_, ok := tplData["simulate_an"]
	if ok {
		sinAnHost, ok := tplData["sim_an_host"]
		if ok {
			comm.TestCommunication(s.CommHandlerCreator, sinAnHost)
		}
	}

	_, ok = tplData["json"]
	if ok {
		//err error, response http.ResponseWriter, data interface{}
		serveJSON(nil, response, map[string]interface{}{})
	} else {
		serveHTMLTpl(fmt.Sprintf("communication%sindex.tpl.html", string(os.PathSeparator)), response, tplData, true)
	}

}

func (s *SIUIAPIServerType) APIRestartCommunication(response http.ResponseWriter, request *http.Request) {
	comm.ReStartEquipmentCommunication(s.CommHandlerCreator)

	serveJSON(nil, response, nil)
	return
}

func (s *SIUIAPIServerType) APIStopCommunication(response http.ResponseWriter, request *http.Request) {
	comm.EndEquipmentCommunication()

	serveJSON(nil, response, nil)
	return
}
