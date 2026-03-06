package implementation

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"wisemed-labreaders/sqlitewrapper"
)

var systemDPath string
var implAppVersion string
var implAppAnalyzerName string

func CheckParameters(appVersion string, appAnalyzerName string) error {
	implAppVersion = appVersion
	implAppAnalyzerName = appAnalyzerName
	var appVer stringFlag
	var appHelp stringFlag
	var installSrv stringFlag

	flag.StringVar(&sqlitewrapper.SQLITEAPPParams.SqlitePath, "dbpath", "", "modify sqlite database path")
	flag.StringVar(&sqlitewrapper.SQLITEAPPParams.ResetCerts, "resetcerts", "default 0", "1 to reset certificates path")
	flag.StringVar(&systemDPath, "systemd-path", "", "Service location directory on your system")

	flag.Var(&installSrv, "install", "Install service daemon")
	flag.Var(&appVer, "version", "Display program version")
	flag.Var(&appHelp, "help", "Display program help")

	flag.Parse()

	if sqlitewrapper.SQLITEAPPParams.SqlitePath == "" {
		var err error
		sqlitewrapper.SQLITEAPPParams.SqlitePath, err = filepath.Abs(filepath.Dir(os.Args[0])) //get the current working directory
		if err != nil {
			sqlitewrapper.SQLITEAPPParams.SqlitePath = ""
		}
		sqlitewrapper.SQLITEAPPParams.SqlitePath = fmt.Sprintf("%s%s", sqlitewrapper.SQLITEAPPParams.SqlitePath, string(os.PathSeparator))
	}

	if appVer.set {
		fmt.Printf("\vVersion %s", appVersion)
		os.Exit(0)
	}

	if appHelp.set {
		fmt.Printf("\vVersion %s", appVersion)
		flag.Usage()
		fmt.Println("\nService related stuff")
		fmt.Println(howToSteps())
		os.Exit(0)
	}
	if installSrv.set {
		fmt.Printf("\vVersion %s", appVersion)
		createService()
		os.Exit(0)
	}

	return nil
}
func howToSteps() string {
	return `1. Reload the systemctl daemon:
sudo systemctl daemon-reload

2. Start your service with:
sudo systemctl start ` + getServiceName() + `

3. Check your service status with:
sudo systemctl status ` + getServiceName() + `

4. To enable your service on every reboot
sudo systemctl enable ` + getServiceName() + `

5. To disable your service on every reboot
sudo systemctl disable ` + getServiceName() + `

`
}

func getServiceName() string {
	return fmt.Sprintf("wm-%s.service", strings.ToLower(strings.ReplaceAll(strings.ToLower(implAppAnalyzerName), " ", "-")))
}
func createService() {
	//argsWithProg := os.Args
	serviceName := getServiceName()
	servicePath, err := filepath.Abs(filepath.Dir(os.Args[0]))

	if err != nil {
		servicePath = "."
	}
	//serviceFileName := filepath.Base(argsWithProg[0])

	if systemDPath == "" {
		systemDPath = "/etc/systemd/system"
	}

	serviceFullPath := fmt.Sprintf("%s%c%s", systemDPath, os.PathSeparator, serviceName)
	fmt.Printf("\nInstalling new service as: %s\n", serviceFullPath)
	f, err := os.Create(serviceFullPath)
	if err != nil {
		fmt.Printf("\nFailed to install the service: %s\n", err.Error())
		return
	}
	defer f.Close()

	f.WriteString(`[Unit]
Description=` + strings.ToLower(implAppAnalyzerName) + ` WiseMED LabReader Service

[Service]
User=root
WorkingDirectory=` + servicePath + `
ExecStart=` + servicePath + string(os.PathSeparator) + filepath.Base(os.Args[0]) + `
StandardOutput=file:` + servicePath + string(os.PathSeparator) + `wm-cobas.log
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
`)

	fmt.Printf("\nService installed successfully!\n\nNext steps:\n%s", howToSteps())
	os.Exit(0)
}

type stringFlag struct {
	set   bool
	value string
}

func (sf *stringFlag) Set(x string) error {
	sf.value = x
	sf.set = true
	return nil
}

func (sf *stringFlag) String() string {
	return sf.value
}
