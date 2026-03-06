package sqlitewrapper

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"wisemed-labreaders/general"
)

type SQLKnownTestsType int

const (
	KATypeQuantitative SQLKnownTestsType = iota + 1
	KATypeQalitative
)

type SQLKnownTestsFormatting int

const (
	KAFormatingNoFormat SQLKnownTestsFormatting = iota + 1
	KAFormatingToNumberNoDecimals
	KAFormatingToNumber1Decimals
	KAFormatingToNumber2Decimals
	KAFormatingToNumber3Decimals
	KAFormatingToNumber4Decimals
)

type SQLKnownTestTrans struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type SQLKnownTestQuery struct {
	Id     int    `json:"known_test_id"`
	Active string `json:"kt_active"`
	Tag    string `json:"kt_tag"`
	Code   string `json:"kt_code"`
}

type SQLKnownTest struct {
	Id                   int                     `json:"known_test_id"`
	Active               int                     `json:"kt_active"`
	Tag                  string                  `json:"kt_tag"`
	Code                 string                  `json:"kt_code"`
	Details              string                  `json:"kt_details"`
	ResultType           SQLKnownTestsType       `json:"kt_result_type"`
	ResultFormatting     SQLKnownTestsFormatting `json:"kt_result_formatting"`
	ResultWeighting      float32                 `json:"kt_result_weighting"`
	ResultTransformation []SQLKnownTestTrans     `json:"kt_result_transformation"`
	ResultMeasureUnit    string                  `json:"kt_result_measure_unit"`
	ResultReagentsSet    string                  `json:"kt_result_reagents_set"`
}

var knownTestsTablesChecked bool = false

/** =================================================================================================== **/

func (sqlkt *SQLKnownTest) Copy(from *SQLKnownTest) {
	sqlkt.Id = from.Id
	sqlkt.Active = from.Active
	sqlkt.Tag = from.Tag
	sqlkt.Code = from.Code
	sqlkt.Details = from.Details
	sqlkt.ResultType = from.ResultType
	sqlkt.ResultFormatting = from.ResultFormatting
	sqlkt.ResultWeighting = from.ResultWeighting
	sqlkt.ResultTransformation = from.ResultTransformation
	sqlkt.ResultMeasureUnit = from.ResultMeasureUnit
	sqlkt.ResultReagentsSet = from.ResultReagentsSet
}
func (sqlkt *SQLKnownTest) ParseFromRawData(rawData map[string]interface{}) error {
	if len(rawData) != 11 {
		return errors.New("RawData not matching the SQLKnownTest type")
	}

	sqlkt.Id = rawData["known_test_id"].(int)
	sqlkt.Active = rawData["kt_active"].(int)
	sqlkt.Tag = rawData["kt_tag"].(string)
	sqlkt.Code = rawData["kt_code"].(string)
	sqlkt.Details = rawData["kt_details"].(string)
	sqlkt.ResultType = rawData["kt_result_type"].(SQLKnownTestsType)
	sqlkt.ResultFormatting = rawData["kt_result_formatting"].(SQLKnownTestsFormatting)
	sqlkt.ResultWeighting = rawData["kt_result_weighting"].(float32)
	sqlkt.ResultMeasureUnit = rawData["kt_result_measure_unit"].(string)
	sqlkt.ResultReagentsSet = rawData["kt_result_reagents_set"].(string)

	if tmpVal, ok := rawData["kt_result_transformation"]; ok {
		trans := []SQLKnownTestTrans{}
		transJSON := tmpVal.(string)
		json.Unmarshal([]byte(transJSON), &trans)
		sqlkt.ResultTransformation = trans
	}

	return nil
}

func (sqlkt *SQLKnownTest) ParseFromMap(mapData map[string]string) error {
	var err error
	sqlkt.Id, err = strconv.Atoi(mapData["known_test_id"])
	if err != nil {
		sqlkt.Id = 0
	}
	sqlkt.Active, err = strconv.Atoi(mapData["kt_active"])
	if err != nil {
		sqlkt.Active = 0
	}
	sqlkt.Tag = mapData["kt_tag"]
	sqlkt.Code = mapData["kt_code"]
	sqlkt.Details = mapData["kt_details"]

	tmpId, err := strconv.Atoi(mapData["kt_result_type"])
	if err != nil {
		tmpId = 0
	}
	sqlkt.ResultType = SQLKnownTestsType(tmpId)
	tmpId, err = strconv.Atoi(mapData["kt_result_formatting"])
	if err != nil {
		tmpId = 0
	}
	sqlkt.ResultFormatting = SQLKnownTestsFormatting(tmpId)
	rw, err := strconv.ParseFloat(mapData["kt_result_weighting"], 32)
	if err != nil {
		rw = 0
	}
	sqlkt.ResultWeighting = float32(rw)
	sqlkt.ResultMeasureUnit = mapData["kt_result_measure_unit"]
	sqlkt.ResultReagentsSet = mapData["kt_result_reagents_set"]

	trans := []SQLKnownTestTrans{}

	json.Unmarshal([]byte(mapData["kt_result_transformation"]), &trans)
	sqlkt.ResultTransformation = trans

	return nil
}

func (sqlkt *SQLKnownTest) ToMap() (map[string]string, error) {
	tmpMap := make(map[string]string)
	tmpMap["known_test_id"] = strconv.Itoa(sqlkt.Id)
	tmpMap["kt_active"] = strconv.Itoa(sqlkt.Active)
	tmpMap["kt_tag"] = sqlkt.Tag
	tmpMap["kt_code"] = sqlkt.Code
	tmpMap["kt_details"] = sqlkt.Details
	tmpMap["kt_result_type"] = strconv.Itoa(int(sqlkt.ResultType))
	tmpMap["kt_result_formatting"] = strconv.Itoa(int(sqlkt.ResultFormatting))
	tmpMap["kt_result_weighting"] = fmt.Sprintf("%f", sqlkt.ResultWeighting)
	tmpMap["kt_result_measure_unit"] = sqlkt.ResultMeasureUnit
	tmpMap["kt_result_reagents_set"] = sqlkt.ResultReagentsSet

	jsonTxt, err := json.Marshal(sqlkt.ResultTransformation)
	if err != nil {
		jsonTxt = []byte("[]")
	}
	tmpMap["kt_result_transformation"] = string(jsonTxt)

	return tmpMap, nil
}

func GetKTQueryInterface() []interface{} {
	return []interface{}{ // Standard MySQL columns
		new(int),                     //known_test_id
		new(int),                     //kt_active
		new(string),                  //kt_tag
		new(string),                  //kt_code
		new(string),                  //kt_details
		new(SQLKnownTestsType),       //kt_result_type
		new(SQLKnownTestsFormatting), //kt_result_formatting
		new(float32),                 //kt_result_weighting
		new(string),                  //kt_result_transformation
		new(string),                  //kt_result_measure_unit
		new(string),                  //kt_result_reagents_set
	}
}

func CheckKnownTestsTable() error {

	if knownTestsTablesChecked {
		return nil
	}
	OpenSQLLiteDatabase()

	SQLITEDatabase.CheckTableObj("KNOWN_TESTS", "create table KNOWN_TESTS ("+
		"known_test_id INTEGER PRIMARY KEY, "+
		"kt_active int, "+
		"kt_tag TEXT, "+
		"kt_code TEXT, "+
		"kt_details text, "+
		"kt_result_type int, "+
		"kt_result_formatting int, "+
		"kt_result_weighting real, "+
		"kt_result_transformation text,"+
		"kt_result_measure_unit text,"+
		"kt_result_reagents_set text"+
		")")

	SQLITEDatabase.CheckTableObj("KNOWN_TESTS_UNAVAILABILITY", "create table KNOWN_TESTS_UNAVAILABILITY ("+
		"known_test_unav_id INTEGER PRIMARY KEY, "+
		"ktua_known_test_id int, "+
		"ktua_date date, "+
		"ktua_reason text, "+
		"ktua_registered_by_id int, "+
		"ktua_registered_by_name text"+
		")")

	knownTestsTablesChecked = true
	return nil
}

func GetKnownTestsNo() (int, int, error) {
	CheckKnownTestsTable()

	tot, err := SQLITEDatabase.ExecTotalQuery("KNOWN_TESTS", "", nil)
	if err != nil {
		return -1, -1, err
	}
	totActive, err := SQLITEDatabase.ExecTotalQuery("KNOWN_TESTS", "kt_active = ?", 1)
	if err != nil {
		return -1, -1, err
	}

	return tot, totActive, nil
}

func GetKnownTests() ([]SQLKnownTest, error) {
	CheckKnownTestsTable()
	dest := GetKTQueryInterface()

	data, err := SQLITEDatabase.ExecQueryFromTable("KNOWN_TESTS", "select * from KNOWN_TESTS", dest)
	if err != nil {
		return nil, err
	}

	sqlKTS := []SQLKnownTest{}
	for i := range data {
		sqlKT := SQLKnownTest{}
		err := sqlKT.ParseFromRawData(data[i])
		if err != nil {
			return []SQLKnownTest{}, err
		}
		sqlKTS = append(sqlKTS, sqlKT)
	}

	return sqlKTS, nil
}

func GetKnownTest(testId int) (SQLKnownTest, error) {
	kt := SQLKnownTest{}
	ktsts, err := GetKnownTestsQuery(SQLKnownTestQuery{Id: testId})
	if err != nil {
		return kt, err
	}
	if len(ktsts) > 0 {
		kt = ktsts[0]
	}

	return kt, nil
}

func GetKnownTestsQuery(query SQLKnownTestQuery) ([]SQLKnownTest, error) {
	CheckKnownTestsTable()

	dbQuery := ""
	if query.Id > 0 {
		dbQuery += fmt.Sprintf(" and known_test_id='%d'", query.Id)
	}
	if query.Code != "" {
		dbQuery += " and kt_code='" + query.Code + "'"
	}
	if query.Tag != "" {
		dbQuery += " and kt_tag='" + query.Tag + "'"
	}
	if query.Active != "" {
		dbQuery += " and kt_active ='" + query.Active + "'"
	}

	sqlKTArr := []SQLKnownTest{}
	sqlKT := SQLKnownTest{}
	if dbQuery != "" {

		dbQuery = "select * from KNOWN_TESTS where (1=1)" + dbQuery

		dest := GetKTQueryInterface()
		data, err := SQLITEDatabase.ExecQueryFromTable("KNOWN_TESTS", dbQuery, dest)
		if err != nil {
			return nil, err
		}

		if len(data) > 0 {
			err := sqlKT.ParseFromRawData(data[0])
			if err != nil {
				return nil, err
			}
			sqlKTArr = append(sqlKTArr, sqlKT)
		}
	}

	return sqlKTArr, nil
}

func SaveKnownTest(ktId int, ktData SQLKnownTest) error {
	CheckKnownTestsTable()
	stmt, err := SQLITEDatabase.DbObj.Prepare("select * from KNOWN_TESTS where known_test_id = ?")
	if err != nil {
		return err
	}

	rows, err := stmt.Query(ktId)
	if err != nil {
		return err
	}
	stmt.Close()

	var execParams []interface{}

	jsonTransformation, err := json.Marshal(ktData.ResultTransformation)
	if err != nil {
		jsonTransformation = []byte("[]")
	}

	if rows.Next() {
		//update
		if debugSQLWrapperLevel > 10 {
			fmt.Println("Updating KT")
		}
		stmt, err = SQLITEDatabase.DbObj.Prepare("update KNOWN_TESTS set kt_active = ?,kt_tag=?,kt_code=?,kt_details=?,kt_result_type=?,kt_result_formatting=?,kt_result_weighting=?,kt_result_transformation=?,kt_result_measure_unit=?,kt_result_reagents_set=?  where known_test_id = ?")

		if err != nil {
			return err
		}
		execParams = []interface{}{ktData.Active, ktData.Tag, ktData.Code, ktData.Details, ktData.ResultType, ktData.ResultFormatting, ktData.ResultWeighting, string(jsonTransformation), ktData.ResultMeasureUnit, ktData.ResultReagentsSet, ktId}

	} else {
		//insert
		if debugSQLWrapperLevel > 10 {
			fmt.Println("Inserting KT")
		}
		stmt, err = SQLITEDatabase.DbObj.Prepare("insert into KNOWN_TESTS(kt_active, kt_tag, kt_code,kt_details,kt_result_type,kt_result_formatting,kt_result_weighting,kt_result_transformation,kt_result_measure_unit, kt_result_reagents_set) values(?,?, ?, ?, ?, ?, ?, ?,?,?)")
		if err != nil {
			return err
		}
		execParams = []interface{}{ktData.Active, ktData.Tag, ktData.Code, ktData.Details, ktData.ResultType, ktData.ResultFormatting, ktData.ResultWeighting, string(jsonTransformation), ktData.ResultMeasureUnit, ktData.ResultReagentsSet}
	}
	rows.Close()

	_, err = stmt.Exec(execParams...)
	if err != nil {
		return err
	}

	defer stmt.Close()
	return nil
}

func SaveKnownTestsBulk(queue *general.ObjectQueue) error {
	CheckKnownTestsTable()
	for queue.Len() > 0 {
		tmpObj, err := queue.Pop()
		if err != nil {
			fmt.Printf("Error on save known test: %v\n", err)
			return err
		}
		tmpKT, ok := tmpObj.(SQLKnownTest)
		if ok {
			if debugSQLWrapperLevel > 10 {
				fmt.Printf("New known test result to save %v\n", tmpKT)
			}
			err = SaveKnownTest(tmpKT.Id, tmpKT)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func SaveKnownTestFromMap(ktId int, ktData map[string]string) error {
	ktObj := SQLKnownTest{}
	ktObj.ParseFromMap(ktData)
	return SaveKnownTest(ktId, ktObj)
}

func DeleteKnownTest(ktId int) error {
	CheckKnownTestsTable()
	if debugSQLWrapperLevel > 10 {
		fmt.Println("Deleting KT")
	}
	stmt, err := SQLITEDatabase.DbObj.Prepare("delete from KNOWN_TESTS where known_test_id = ?")
	defer stmt.Close()

	if err != nil {
		return err
	}
	execParams := []interface{}{ktId}

	_, err = stmt.Exec(execParams...)
	if err != nil {
		return err
	}
	return nil
}
