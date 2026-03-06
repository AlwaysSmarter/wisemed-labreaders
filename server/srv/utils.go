package srv

import (
	"encoding/json"
	"fmt"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/dgrijalva/jwt-go/request"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
	"wisemed-labreaders/comm"
	"wisemed-labreaders/config"
	"wisemed-labreaders/general"
	"wisemed-labreaders/sqlitewrapper"

	//"path/filepath"
	"bytes"
	"reflect"
	"strings"
)

var APP_ERROR_TXT string

type StatusErr struct {
	Error string `json:"error"`
}

type SIUIAPIServerType struct {
	Router             *mux.Router
	Config             config.WMLRAPIConfigServer
	ImplementationDir  string
	CommHandlerCreator config.CreateProtocolHandler
	SessManager        *SessionManager
	Session            Session
	LoggedinUser       *general.WMLRLoginInfo
	OtherConfig        func(tplDataNoConfig map[string]string) string
	DefaultKTData      func() []map[string]string
	WSClient           *WSClient
}

func buildAceJsonResponse(success bool, data interface{}, rows interface{}, err error) (map[string]interface{}, error) {
	resp := map[string]interface{}{"success": success}

	if success {
		rowsType := reflect.ValueOf(rows)
		//fmt.Println(rowsType)
		//fmt.Println(rowsType.Kind())
		if rowsType.Kind() == reflect.Array || rowsType.Kind() == reflect.Slice {
			if rowsType.Len() > 0 {
				resp["totalCount"] = rowsType.Len()
				resp["rows"] = rows
			} else {
				resp["totalCount"] = 0
			}
		}
	} else {
		resp["error"] = err.Error()
	}

	if data != nil {
		resp["data"] = data
	}

	return resp, nil
}

func respondWithError(err error, statusCode int, response http.ResponseWriter) {
	general.PrettyPrint(false, err)
	errStatus := StatusErr{
		Error: fmt.Sprintf("%s", err),
	}
	response.WriteHeader(statusCode)

	aceJson, _ := buildAceJsonResponse(false, nil, errStatus, err)

	if tmpErr := json.NewEncoder(response).Encode(aceJson); tmpErr != nil {
		general.PrettyPrint(false, tmpErr)
		response.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func replaceHTMLTpl(content []byte, tplData map[string]string) []byte {

	if tplData["APP_ERROR_TXT"] == "" && APP_ERROR_TXT != "" {
		tplData["APP_ERROR_TXT"] = APP_ERROR_TXT
	}
	for key, val := range tplData {
		content = bytes.Replace(content, []byte(fmt.Sprintf("{{%s}}", key)), []byte(val), -1)
	}

	m1 := regexp.MustCompile(`\{\{([A-Za-z\-\_]*)\}\}`)
	contentStr := string(content)

	contentStr = m1.ReplaceAllString(contentStr, "")

	return []byte(contentStr)
}

func returnTPLDirectory(tplPath string, includeFullPath bool) string {
	directoryTpls := ""
	var err error
	if includeFullPath {
		directoryTpls, err = filepath.Abs(filepath.Dir(os.Args[0])) //get the current working directory
		if err != nil {
			directoryTpls = ""
		}
	}

	return fmt.Sprintf("%s%s%s%s", strings.TrimRight(directoryTpls, string(os.PathSeparator)), string(os.PathSeparator), tplPath, string(os.PathSeparator))
}
func returnParsedHTMLTplFile(tplName string, tplData map[string]string, includeFullPath bool) ([]byte, error) {
	directoryTpls := returnTPLDirectory("tpl", includeFullPath)

	//response.WriteHeader(200);
	b, err := ioutil.ReadFile(directoryTpls + tplName) // just pass the file name
	if err != nil {
		return nil, err
	}

	return replaceHTMLTpl(b, tplData), nil
}
func serveJSON(err error, response http.ResponseWriter, data interface{}) {
	general.PrettyPrint(false, err)
	errStatus := StatusErr{}
	success := true
	if err != nil {
		errStatus = StatusErr{
			Error: fmt.Sprintf("%s", err),
		}
		success = false
	}
	response.WriteHeader(http.StatusOK)

	aceJson, _ := buildAceJsonResponse(success, data, errStatus, err)

	if tmpErr := json.NewEncoder(response).Encode(aceJson); tmpErr != nil {
		general.PrettyPrint(false, tmpErr)
		response.WriteHeader(http.StatusInternalServerError)
		return
	}
}
func serveHTMLTpl(templateName string, response http.ResponseWriter, tplData map[string]string, includeFullPath bool) {

	parsedTplFile, err := returnParsedHTMLTplFile("header.tpl.html", tplData, true)
	if err != nil {
		respondWithError(err, http.StatusBadRequest, response)
		return
	}

	response.Write(parsedTplFile)
	parsedTplFile, err = returnParsedHTMLTplFile(templateName, tplData, includeFullPath)
	if err != nil {
		respondWithError(err, http.StatusBadRequest, response)
		return
	}

	response.Write(parsedTplFile)
	parsedTplFile, err = returnParsedHTMLTplFile("footer.tpl.html", tplData, true)
	if err != nil {
		respondWithError(err, http.StatusBadRequest, response)
		return
	}

	response.Write(parsedTplFile)
}

func (s *SIUIAPIServerType) SetLoggedinUser(lu *general.WMLRLoginInfo) {
	s.LoggedinUser = lu
	general.LoggedInUser = lu
}
func (s *SIUIAPIServerType) BroadcastWMMessage(msg map[string]interface{}) {
	b, err := json.Marshal(msg)
	log.Printf("%v", b)
	if err != nil {
		return
	}
	//s.WSHub.BroadcatMessage(b)
}
func (s *SIUIAPIServerType) APIGetHeader(response http.ResponseWriter, request *http.Request) {
	u, err := url.Parse(request.Referer())
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%v", u)
	response.Header().Set("Access-Control-Allow-Origin", "*") // u.Scheme+"://"+u.Host)
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Access-Control-Allow-Credentials", "true")
	response.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST")
	response.Header().Set("Access-Control-Allow-Headers", "Content-Type, Depth, User-Agent, X-File-Size, X-Requested-With, If-Modified-Since, X-File-Name, Cache-Control")

}

func includeDBConfigInTplData(tplData map[string]string) {
	for key, val := range config.DBConfiguration {
		_, ok := tplData[key]
		if !ok {
			tplData[key] = val
		}
	}
	tplData["APP_SERIAL_NO"] = config.ServerConfiguration.APPSerialNo
	tplData["APP_ANALYZER_NAME"] = config.ServerConfiguration.APPAnalyzerName
	tplData["APP_ANALYZER_API_KEY"] = config.ServerConfiguration.APPAPIKey
	tplData["APP_ANALYZER_TYPE"] = string(config.ServerConfiguration.APPAnalyzerType)
	tplData["APP_ANALYZER_TYPE_NAME"] = config.ServerConfiguration.APPAnalyzerType.String()
	tplData["APP_ANALYZER_TYPE_ICON"] = config.ServerConfiguration.APPAnalyzerType.Icon()
}
func parseParameters(server *SIUIAPIServerType, request *http.Request, includeDBConfig bool) map[string]string {
	err := request.ParseForm()
	if err != nil {
		panic(err)
	}

	tplData := make(map[string]string)
	for key, val := range request.Form {
		tplData[key] = strings.Join(val, "")
	}
	if includeDBConfig {
		includeDBConfigInTplData(tplData)
	}

	tplData["APP_SERIAL_NO"] = config.ServerConfiguration.APPSerialNo
	tplData["APP_ANALYZER_NAME"] = config.ServerConfiguration.APPAnalyzerName
	tplData["APP_ANALYZER_API_KEY"] = config.ServerConfiguration.APPAPIKey
	tplData["APP_ANALYZER_TYPE"] = string(config.ServerConfiguration.APPAnalyzerType)
	tplData["APP_ANALYZER_TYPE_NAME"] = config.ServerConfiguration.APPAnalyzerType.String()
	tplData["APP_ANALYZER_TYPE_ICON"] = config.ServerConfiguration.APPAnalyzerType.Icon()
	currentTime := time.Now()
	tplData["APP_DATE"] = currentTime.Format("02/01/2006")
	if server.LoggedinUser != nil {
		tplData["APP_LOGGEDIN_USER"] = fmt.Sprintf("%s %s", server.LoggedinUser.FirstName, server.LoggedinUser.LastName)
	}
	urlScheme := "http://%s"
	if request.TLS != nil && request.TLS.Version > 0 {
		urlScheme = "https://%s"
	}
	tplData["APP_PATH"] = fmt.Sprintf(urlScheme, request.Host)
	return tplData
}
func RegisterAppHtmlTplError(errTxt string, toTplData map[string]string, request ...*http.Request) {
	if toTplData == nil {
		APP_ERROR_TXT = errTxt
	} else {
		toTplData["APP_ERROR_TXT"] = errTxt
	}

	for _, val := range request {
		val.Form.Set("APP_ERROR_TXT", errTxt)
	}
}

func (s *SIUIAPIServerType) APIMenu(response http.ResponseWriter, request *http.Request) {
	tplData := parseParameters(s, request, true)

	fs, err := sqlitewrapper.GetFilesStats(time.Now().Format("2006-01-02"))
	if err != nil {
		respondWithError(err, http.StatusInternalServerError, response)
		return
	}

	commActive := "NO"
	if comm.IsEquipmentCommunicationActive() {
		commActive = "YES"
	}

	tplData["STATS_FINALIZED_PATIENTS"] = strconv.Itoa(fs.FinalizedPatients)
	tplData["STATS_PROGRAMMED_PATIENTS"] = strconv.Itoa(fs.ProgrammedPatients)
	tplData["STATS_ANALISYS_IN_WORK"] = strconv.Itoa(fs.AnalisysInWork)
	tplData["STATS_WORKING_CAP_TOT"] = strconv.Itoa(fs.WorkingCapabilityTotal)
	tplData["STATS_WORKING_CAP_ACTIVE"] = strconv.Itoa(fs.WorkingCapabilityActive)
	tplData["STATUS_ANALYZER_CONNECTED"] = commActive
	serveHTMLTpl("main.tpl.html", response, tplData, true)
}

func (s *SIUIAPIServerType) APIFileServer(response http.ResponseWriter, request *http.Request) {
	defer general.MonitorFunc("api gte file from uploads")()

	filePath := strings.TrimLeft(request.URL.Path, "/uploads")
	filePath = strings.TrimRight(filePath, "/")
	realUploadsPath, err := ReturnUploadsDir(s.Config.UploadsPath, filePath)
	if err != nil {
		respondWithError(err, http.StatusInternalServerError, response)
		return
	}
	http.ServeFile(response, request, realUploadsPath)
}

// Will add authentication check to a handler.
func (s *SIUIAPIServerType) checkAuthWrapper(f func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := url.Parse(r.Referer())
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("%v", u)
		w.Header().Set("Access-Control-Allow-Origin", "*") //  u.Scheme+"://"+u.Host)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Depth, User-Agent, X-File-Size, X-Requested-With, If-Modified-Since, X-File-Name, Cache-Control")

		//if err := s.CheckAuth(w, r); err != nil {
		//	return
		//}

		f(w, r)
		return
	}
}

// Will add authentication check to a handler.
func (s *SIUIAPIServerType) checkHTMLAuthWrapper(f func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := url.Parse(r.Referer())
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("%v", u)
		w.Header().Set("Access-Control-Allow-Origin", "*") // u.Scheme+"://"+u.Host)
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Depth, User-Agent, X-File-Size, X-Requested-With, If-Modified-Since, X-File-Name, Cache-Control")
		//Verify if I am logged in  - if not - try to login
		s.Session = s.SessManager.SessionStart(w, r)
		user := s.Session.Get("loggedinuserdata")
		if user != nil {
			li := general.WMLRLoginInfo{}
			li.ParseFromStrMap(user.(map[string]string))
			s.SetLoggedinUser(&li)
		} else {
			s.SetLoggedinUser(nil)
			if !config.IsMissingAnyFirstRunInfo() {
				s.APILogin(w, r)
				return
			}
		}

		//if err := s.CheckAuth(w, r); err != nil {
		//	return
		//}
		serveFn := true

		tmpInt := config.ReturnIntOrZero(string(config.ServerConfiguration.WMLREquipmentId))
		if tmpInt <= 0 {
			//Try to initialize the first run of WMR
			serveFn = s.APIInitialization(w, r, false)
		} else {

		}

		if serveFn {
			/**
			 * If I am logged in I will follow the request and serve the called function, otherwise I have to login
			 */
			if s.LoggedinUser != nil {
				f(w, r)
			} else {
				/**
				 * I have to log in now
				 */
				s.APILogin(w, r)
			}
		}

		return
	}
}

func (s *SIUIAPIServerType) checkHTMLAPIAuthWrapper(f func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := request.ParseFromRequest(r, request.AuthorizationHeaderExtractor,
			func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				signature := []byte("47c490bb-c9e4-47aa-re33-ebd2a1add35e")
				return signature, nil
			},
		)
		if err != nil || !token.Valid {
			fmt.Printf("Authentication failed " + err.Error())
			w.WriteHeader(http.StatusForbidden)
			return
		}
		claims := token.Claims.(jwt.MapClaims)
		r.Header.Set("caller_type", claims["caller_type"].(string))
		r.Header.Set("caller_name", claims["caller_name"].(string))
		//verify caller ID
		_, err = strconv.Atoi(claims["caller_id"].(string))
		if err != nil {
			respondWithError(fmt.Errorf("unexpected caller ID: %v", claims["caller_id"]), http.StatusBadRequest, w)
			return
		}
		r.Header.Set("caller_id", claims["caller_id"].(string))

		u, err := url.Parse(r.Referer())
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("%v", u)
		w.Header().Set("Access-Control-Allow-Origin", "*") // u.Scheme+"://"+u.Host)
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Depth, User-Agent, X-File-Size, X-Requested-With, If-Modified-Since, X-File-Name, Cache-Control")
		s.Session = s.SessManager.SessionStart(w, r)
		li := general.WMLRLoginInfo{Login: claims["caller_name"].(string)}
		s.SetLoggedinUser(&li)

		f(w, r)

		return
	}
}

// Will add authentication check to a handler.
func (s *SIUIAPIServerType) allowJSOriginWrapper(f func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := url.Parse(r.Referer())
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("%v", u)
		w.Header().Set("Access-Control-Allow-Origin", "*") // u.Scheme+"://"+u.Host)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Depth, User-Agent, X-File-Size, X-Requested-With, If-Modified-Since, X-File-Name, Cache-Control")

		f(w, r)
		return
	}
}

//authKeyOne := securecookie.GenerateRandomKey(64)
//encryptionKeyOne := securecookie.GenerateRandomKey(32)
//
//var store = sessions.NewCookieStore([]byte(os.Getenv("SESSION_KEY")))
//
//func MyHandler(w http.ResponseWriter, r *http.Request) {
//	// Get a session. We're ignoring the error resulted from decoding an
//	// existing session: Get() always returns a session, even if empty.
//	session, _ := store.Get(r, "session-name")
//	// Set some session values.
//	session.Values["foo"] = "bar"
//	session.Values[42] = 43
//	// Save it before we write to the response/return from the handler.
//	err := session.Save(r, w)
//	if err != nil {
//		http.Error(w, err.Error(), http.StatusInternalServerError)
//		return
//	}
//}

func ReturnUploadsDir(uploadsPath, suffix string) (string, error) {
	if uploadsPath == "" {
		path, err := os.Getwd()
		if err != nil {
			return "", err
		}
		uploadsPath = fmt.Sprintf("%s%s%s", strings.TrimRight(path, string(os.PathSeparator)), string(os.PathSeparator), "uploads")
	}
	uploadsPath = fmt.Sprintf("%s%s%s", strings.TrimRight(uploadsPath, string(os.PathSeparator)), string(os.PathSeparator), suffix)

	if _, err := os.Stat(uploadsPath); os.IsNotExist(err) {
		if err = os.MkdirAll(uploadsPath, 0700); err != nil {
			log.Println(err)
			return "", err
		}
	}

	return uploadsPath, nil
}
