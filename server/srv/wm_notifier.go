package srv

import (
	"net/http"
)

func (s *SIUIAPIServerType) APINotifier(response http.ResponseWriter, request *http.Request, withoutAnalysis bool) (bool, error) {
	return notifyOnlineStatus(request, withoutAnalysis)
}

func notifyOnlineStatus(request *http.Request, withoutAnalysis bool) (bool, error) {
	//try to initialize the reader here
	//tplData := make(map[string]string)

	//try to initialize the equipment in WiseMED and receive and ID for it
	_, commParamsModified, err := tryToInitializeOnApi(true)
	return commParamsModified, err
}
