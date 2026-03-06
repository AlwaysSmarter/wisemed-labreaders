package config

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	configdir "github.com/kirsle/configdir"
	"log"
	"strings"
	"wisemed-labreaders/sqlitewrapper"
)

var ServerConfiguration WMLRAPIConfigServer

var DBConfiguration map[string]string

var APPSerialNo string
var APPAnalyzerName string
var APPAnalyzerType WMLRType
var APPAPIKey string
var APPDBSuffixPath = ""
var configPath string

const serialSecret = "abc&1*~#^2^#s0^=)^^7%b34"

var serialBytes = []byte{35, 46, 57, 24, 85, 35, 24, 74, 87, 35, 88, 98, 66, 32, 14, 05}

func getConfigPath(path string) string {
	if configPath == "" {
		cfgPath := configdir.LocalConfig(path)
		if len(cfgPath) <= 0 {
			log.Fatal("Cannot get system config path")
		}
		err := configdir.MakePath(cfgPath) // Ensure it exists.
		if err != nil {
			log.Fatal(err)
		}
		configPath = cfgPath
	}

	log.Print("Config path:", configPath)
	return configPath
}

func IsMissingAnyFirstRunInfo() bool {
	if ServerConfiguration.WMAPIIP == "" ||
		ServerConfiguration.WMAPIKey == "" ||
		ServerConfiguration.WMAPIPort == "" ||
		ServerConfiguration.WMAPIProtocol == "" ||
		ServerConfiguration.WMAPIPath == "" {
		return true
	}
	return false
}
func SetAPPDBSuffixPath(newPath string) {
	APPDBSuffixPath = newPath
	sqlitewrapper.SQLITEAPPParams.DBSufixPath = APPDBSuffixPath
}
func ReadConfig() {
	ServerConfiguration = readServerConfig()
}

func readServerConfig() WMLRAPIConfigServer {
	var srvCfg WMLRAPIConfigServer
	var err error
	DBConfiguration, err = sqlitewrapper.GetConfigurationData()
	if err != nil {
		log.Fatal(err)
	}

	dbCfgJson, err := json.Marshal(DBConfiguration)
	if err != nil {
		log.Fatal(err)
	}
	json.Unmarshal(dbCfgJson, &srvCfg)

	if srvCfg.Port == "" {
		log.Fatal("Unknown reader API port")
	}
	if srvCfg.Address == "" {
		log.Fatal("Unknown reader API address")
	}
	if srvCfg.HTTPSCert == "" {
		log.Fatal("Unknown reader API https certificate")
	}
	if srvCfg.HTTPSCert == "" {
		log.Fatal("Unknown reader API https certificate key")
	}

	srvCfg.APPSerialNo = APPSerialNo
	srvCfg.APPAnalyzerName = APPAnalyzerName
	srvCfg.APPAnalyzerType = APPAnalyzerType
	srvCfg.APPAPIKey = APPAPIKey

	srvCfg.OtherConfig = map[string]string{}
	for key, val := range DBConfiguration {
		if strings.HasPrefix(key, "othercfg_") {
			srvCfg.OtherConfig[key] = val
		}
	}
	return srvCfg
}

func Encrypt(text string) (string, error) {
	block, err := aes.NewCipher([]byte(serialSecret))
	if err != nil {
		return "", err
	}
	plainText := []byte(text)
	cfb := cipher.NewCFBEncrypter(block, serialBytes)
	cipherText := make([]byte, len(plainText))
	cfb.XORKeyStream(cipherText, plainText)
	return encode(cipherText), nil
}

func Decrypt(text string) string {
	block, err := aes.NewCipher([]byte(serialSecret))
	if err != nil {
		return ""
	}
	cipherText := decode(text)
	cfb := cipher.NewCFBDecrypter(block, serialBytes)
	plainText := make([]byte, len(cipherText))
	cfb.XORKeyStream(plainText, cipherText)
	return string(plainText)
}

func encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func decode(s string) []byte {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return data
}
