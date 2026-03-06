package sqlitewrapper

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type SQLConfig struct {
	Key string `json:"key"`
	Val string `json:"val"`
}

var configTableChecked bool = false

func (sqlcof *SQLConfig) parseFromRawData(rawData map[string]interface{}) error {
	if len(rawData) != 2 {
		return errors.New("RawData not matching the SQLConfig type")
	}

	sqlcof.Key = rawData["key"].(string)
	sqlcof.Val = rawData["val"].(string)

	return nil
}
func CheckConfigurationTable() error {
	if configTableChecked {
		return nil
	}

	OpenSQLLiteDatabase()
	SQLITEDatabase.CheckTableObj("CONFIG", "create table CONFIG (key text, val text)")

	if debugSQLWrapperLevel > 2 {
		fmt.Println("CHECK TABLE FIELDS:")
	}
	cft, _ := SQLITEDatabase.ReturnTableFields("CONFIG")
	if debugSQLWrapperLevel > 2 {
		fmt.Println(cft)
	}
	configTableChecked = true
	return nil
}
func GetConfigurationDataJSON() ([]byte, error) {
	cfgData, err := GetConfigurationData()
	if err != nil {
		return nil, err
	}
	return json.Marshal(cfgData)
}
func GetConfigurationDataExt() error {
	dest := []interface{}{ // Standard MySQL columns
		new(string),
		new(string),
	}
	data, err := SQLITEDatabase.ExecQueryFromTable("CONFIG", "select * from CONFIG", dest)
	if err != nil {
		return err
	}

	sqlConfig := SQLConfig{}
	for i := range data {
		sqlConfig.parseFromRawData(data[i])
		if debugSQLWrapperLevel > 2 {
			fmt.Println("ROW ", i)
			fmt.Println(sqlConfig)
		}
	}

	return nil
}
func GetConfigurationData() (map[string]string, error) {
	err := CheckConfigurationTable()
	if err != nil {
		return nil, err
	}

	rows, err := SQLITEDatabase.DbObj.Query("select key, val from CONFIG")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]string)
	var key string
	var val string
	for rows.Next() {
		err = rows.Scan(&key, &val)
		if err != nil {
			return nil, err
		}
		m[key] = val
	}

	addedDefaults := writeConfigDefaults(m)
	if len(addedDefaults) > 0 {
		InsertConfigurationData(addedDefaults)
	}

	return m, nil
}

func InsertConfigurationData(modifiedCfg map[string]string) error {
	stmt, err := SQLITEDatabase.DbObj.Prepare("insert into CONFIG(key, val) values(?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for key, val := range modifiedCfg {
		_, err = stmt.Exec(key, val)
		if err != nil {
			return nil
		}
	}

	return nil
}

func SetConfigurationData(configData map[string]string) error {
	for key, val := range configData {
		err := SetConfigurationPair(key, val)
		if err != nil {
			return err
		}
	}

	return nil
}

func SetConfigurationPair(key string, val string) error {
	stmt, err := SQLITEDatabase.DbObj.Prepare("select * from CONFIG where key = ?")
	if err != nil {
		return err
	}

	rows, err := stmt.Query(key)
	if err != nil {
		return err
	}
	stmt.Close()

	var execParams []interface{}
	if rows.Next() {
		//update
		if debugSQLWrapperLevel > 10 {
			fmt.Println("Updating")
		}
		stmt, err = SQLITEDatabase.DbObj.Prepare("update CONFIG set val = ? where key = ?")
		if err != nil {
			return err
		}
		execParams = []interface{}{val, key}

	} else {
		//insert
		if debugSQLWrapperLevel > 10 {
			fmt.Println("Insert")
		}
		stmt, err = SQLITEDatabase.DbObj.Prepare("insert into CONFIG(key, val) values(?, ?)")
		if err != nil {
			return err
		}
		execParams = []interface{}{key, val}
	}
	rows.Close()

	_, err = stmt.Exec(execParams...)
	if err != nil {
		return err
	}
	defer stmt.Close()
	return nil
}

func writeConfigDefaults(cfg map[string]string) map[string]string {
	modified := make(map[string]string)
	appDir, err := filepath.Abs(filepath.Dir(os.Args[0])) //get the current working directory
	if err != nil {
		appDir = ""
	}
	appDir = fmt.Sprintf("%s%s", appDir, string(os.PathSeparator))

	if cfg["cfg_reader_api_port"] == "" {
		cfg["cfg_reader_api_port"] = "8010"
		modified["cfg_reader_api_port"] = "8010"
	}
	if cfg["cfg_reader_api_address"] == "" {
		cfg["cfg_reader_api_address"] = "127.0.0.1"
		modified["cfg_reader_api_address"] = "127.0.0.1"
	}
	if cfg["cfg_reader_api_cert"] == "" || SQLITEAPPParams.ResetCerts == "1" {
		cfg["cfg_reader_api_cert"] = fmt.Sprintf("%scerts%sdomain.crt", appDir, string(os.PathSeparator))
		modified["cfg_reader_api_cert"] = fmt.Sprintf("%scerts%sdomain.crt", appDir, string(os.PathSeparator))
	}

	if cfg["cfg_reader_api_cert_privatekey"] == "" || SQLITEAPPParams.ResetCerts == "1" {
		cfg["cfg_reader_api_cert_privatekey"] = fmt.Sprintf("%scerts%sdomain.key", appDir, string(os.PathSeparator))
		modified["cfg_reader_api_cert_privatekey"] = fmt.Sprintf("%scerts%sdomain.key", appDir, string(os.PathSeparator))
	}

	if cfg["cfg_reader_api_downloads_dir"] == "" {
		cfg["cfg_reader_api_downloads_dir"] = "downloads"
		modified["cfg_reader_api_downloads_dir"] = "downloads"
	}
	if cfg["cfg_reader_api_uploads_dir"] == "" {
		cfg["cfg_reader_api_uploads_dir"] = "uploads"
		modified["cfg_reader_api_uploads_dir"] = "uploads"
	}

	//WebService defaults
	if cfg["ip"] == "" {
		cfg["ip"] = "127.0.0.1"
		modified["ip"] = "127.0.0.1"
	}
	if cfg["port"] == "" {
		cfg["port"] = "7777"
		modified["port"] = "7777"
	}
	return modified
}
