package srv

import (
	"context"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"wisemed-labreaders/comm"
	"wisemed-labreaders/config"
	"wisemed-labreaders/general"
)

const useWS = true

func init() {
	//config.ReadConfig()
}

func (s *SIUIAPIServerType) GetServerConfig(appSerialNo, appAnalyzerName string, appAnalyzerType config.WMLRType, appAPIKey string) error {
	//enc, _ := config.Encrypt("864100003964")
	//panic(fmt.Sprintf("ENCRYPTED SERIAL SHOULD BE: %s\n", enc))

	config.APPSerialNo = config.Decrypt(appSerialNo)
	config.SetAPPDBSuffixPath(config.APPSerialNo)
	config.APPAnalyzerName = appAnalyzerName
	config.APPAPIKey = appAPIKey
	config.APPAnalyzerType = appAnalyzerType

	config.ReadConfig()
	//config.ServerConfiguration.APPSerialNo = config.Decrypt(appSerialNo)
	//config.ServerConfiguration.APPAnalyzerName = appAnalyzerName
	//config.ServerConfiguration.APPAPIKey = appAPIKey
	//config.ServerConfiguration.APPAnalyzerType = appAnalyzerType

	s.Config = config.ServerConfiguration

	return nil
}

func (s *SIUIAPIServerType) InitWSHub() {
	if useWS {
		log.Println("Initializing WebSocket Hub")

		ws := NewWSClient(
			config.APPSerialNo,
			"https://local.wisemed.eu/git/wisemed-api/apiv2/ws-endpoint",
			config.APPAPIKey,
		)

		s.WSClient = ws
		go ws.ConnectLoop()
	}
}

func (s *SIUIAPIServerType) InitRouter() {
	s.Router = mux.NewRouter()
}

func (s *SIUIAPIServerType) BuildHandlers() {
	s.Router.Methods("OPTIONS").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			log.Println("Handle preflight here")
			u, err := url.Parse(r.Referer())
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("%v", u)
			w.Header().Set("Access-Control-Allow-Origin", "*") // u.Scheme+"://"+u.Host)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Depth, User-Agent, X-File-Size, X-Requested-With, If-Modified-Since, X-File-Name, Cache-Control, Authorization")

			w.WriteHeader(http.StatusOK)
			return
		})
	s.Router.HandleFunc("/", s.checkHTMLAuthWrapper(s.APIMenu)).Methods("GET")
	s.Router.HandleFunc("/", s.checkHTMLAuthWrapper(s.APIMenu)).Methods("POST")
	s.Router.HandleFunc("/login", s.checkHTMLAuthWrapper(s.APILogin)).Methods("GET")
	s.Router.HandleFunc("/login", s.checkHTMLAuthWrapper(s.APILogin)).Methods("POST")
	s.Router.HandleFunc("/logout", s.checkHTMLAuthWrapper(s.APILogout)).Methods("GET")
	s.Router.HandleFunc("/logout", s.checkHTMLAuthWrapper(s.APILogout)).Methods("POST")
	s.Router.HandleFunc("/config", s.checkHTMLAuthWrapper(s.APIConfig)).Methods("GET")
	s.Router.HandleFunc("/config", s.checkHTMLAuthWrapper(s.APIConfig)).Methods("POST")
	s.Router.HandleFunc("/knowntests", s.checkHTMLAuthWrapper(s.APIGetKnownTests)).Methods("GET")
	s.Router.HandleFunc("/knowntests", s.checkHTMLAuthWrapper(s.APISaveKnownTests)).Methods("POST")
	s.Router.HandleFunc("/reloadktfromanalyzer", s.checkHTMLAuthWrapper(s.APIReloadKnownTestsFromAnalyzer)).Methods("GET")
	s.Router.HandleFunc("/addmissingkt", s.checkHTMLAuthWrapper(s.APIAddMissingDefaultKT)).Methods("GET")
	s.Router.HandleFunc("/addmissingkt", s.checkHTMLAuthWrapper(s.APIAddMissingDefaultKT)).Methods("POST")
	s.Router.HandleFunc("/restartcomm", s.checkHTMLAuthWrapper(s.APIRestartCommunication)).Methods("GET")
	s.Router.HandleFunc("/stopcomm", s.checkHTMLAuthWrapper(s.APIStopCommunication)).Methods("GET")
	s.Router.HandleFunc("/how-to", s.checkHTMLAuthWrapper(s.APIHowTOConfig)).Methods("GET")
	s.Router.HandleFunc("/communication", s.checkHTMLAuthWrapper(s.APICommunication)).Methods("GET")
	s.Router.HandleFunc("/communication", s.checkHTMLAuthWrapper(s.APICommunication)).Methods("POST")
	s.Router.HandleFunc("/api/config", s.checkHTMLAPIAuthWrapper(s.APIAPIGetConfig)).Methods("GET")
	s.Router.HandleFunc("/api/tests", s.checkHTMLAPIAuthWrapper(s.APIAPIGetTests)).Methods("GET")

}

func checkAllowOrigin(origin string) bool {
	RegisterAppHtmlTplError("", nil)
	log.Print("checkAllowOrigin from " + origin)
	return true
}

func checkReqAllowOrigin(r *http.Request, origin string) bool {
	log.Print("checkReqAllowOrigin from " + origin)
	return true
}

func (s *SIUIAPIServerType) Run(appSerialNo, appAnalyzerName string, appAnalyzerType config.WMLRType, appAnalyzerImplDirectory string, appAPIKey string) {
	if err := s.GetServerConfig(appSerialNo, appAnalyzerName, appAnalyzerType, appAPIKey); err != nil {
		log.Fatal(err)
	}
	s.ImplementationDir = appAnalyzerImplDirectory
	s.InitWSHub()
	s.InitRouter()
	s.BuildHandlers()

	path, err := os.Getwd()
	if err != nil {
		general.PrettyPrint(true, "cannot find the current path")
		return
	}

	directoryUploads := fmt.Sprintf("%s%s%s", strings.TrimRight(path, string(os.PathSeparator)), string(os.PathSeparator), "uploads")
	s.Router.PathPrefix("/uploads/").Handler(http.StripPrefix("/uploads/", http.FileServer(http.Dir(directoryUploads))))

	directoryWeb := fmt.Sprintf("%s%s%s", strings.TrimRight(path, string(os.PathSeparator)), string(os.PathSeparator), "web")
	s.Router.PathPrefix("/web/").Handler(http.StripPrefix("/web/", http.FileServer(http.Dir(directoryWeb))))

	diresctoryHowToRes := fmt.Sprintf("%s%s%s%s%s%s%s%s%s/", strings.TrimRight(path, string(os.PathSeparator)), string(os.PathSeparator), "implementation", string(os.PathSeparator), s.ImplementationDir, string(os.PathSeparator), "how-to", string(os.PathSeparator), "res")
	s.Router.PathPrefix("/how-to-res/").Handler(http.StripPrefix("/how-to-res/", http.FileServer(http.Dir(diresctoryHowToRes))))
	diresctoryHowToRes = fmt.Sprintf("%s%s%s%s%s%s%s%s%s/", strings.TrimRight(path, string(os.PathSeparator)), string(os.PathSeparator), "implementation", string(os.PathSeparator), s.ImplementationDir, string(os.PathSeparator), "communication", string(os.PathSeparator), "res")
	s.Router.PathPrefix("/comm-res/").Handler(http.StripPrefix("/comm-res/", http.FileServer(http.Dir(diresctoryHowToRes))))

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Print("CORS - DEFAULTS")
	log.Print(cors.Default())

	CORSHandler := cors.New(cors.Options{
		AllowOriginRequestFunc: checkReqAllowOrigin,
		AllowCredentials:       true,
		OptionsPassthrough:     true,
		Debug:                  true,
		AllowedMethods:         []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:         []string{"Content-Type", "Bearer", "Bearer ", "content-type", "Origin", "Accept", "Authorization"},
	})
	log.Print(CORSHandler)
	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", s.Config.Address, s.Config.Port),
		Handler: CORSHandler.Handler(s.Router),
	}

	go func() {
		if err := srv.ListenAndServeTLS(s.Config.HTTPSCert, s.Config.HTTPSKey); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	log.Printf("\nServer Started on %s:%s", s.Config.Address, s.Config.Port)

	err = comm.ReStartEquipmentCommunication(s.CommHandlerCreator)
	if err != nil {
		log.Print("Communication error: ", err)
	} else {
		log.Print("Communication Started")
	}
	//log.Printf("Starting WS communication to the WiseMED Proxy Server")

	<-done
	log.Print("Server Stopped")
	comm.EndEquipmentCommunication()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		// extra handling here
		time.Sleep(2 * time.Second)
		cancel()
	}()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server Shutdown Failed:%+v", err)
	}
	log.Print("Server Exited Properly")
}
