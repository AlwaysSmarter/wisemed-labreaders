package sqlitewrapper

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"strconv"
	"strings"
	"time"
	"wisemed-labreaders/general"
)

type SQLTest struct {
	Id                     int    `json:"test_id"`
	OrderId                int    `json:"t_order_id"`
	Name                   string `json:"t_name"`
	Code                   string `json:"t_code"`
	Tag                    string `json:"t_tag"`
	SampleVolume           string `json:"t_sample_volume"`
	SelectedToBeProgrammed bool   `json:"t_selected_for_prog"`

	ProgrammedInAnalyzerDateTime string             `json:"t_programmed_in_analyzer_on"`
	ProgrammedInAnalyzerBy       string             `json:"t_programmed_in_analyzer_by"`
	ResultType                   SQLOrderResultType `json:"tr_result_type"`
	Result                       string             `json:"t_curr_result"`
	Raw                          string             `json:"t_curr_raw_result"`
	Interpretation               string             `json:"t_curr_interpretation"`
	ResultReceivedDateTime       string             `json:"t_curr_result_rcvd_on"`
	ResultReceivedBy             string             `json:"t_curr_result_rcvd_by"`
	InsertedDateTime             string             `json:"t_inserted_on"`
	InsertedBy                   string             `json:"t_inserted_by"`
	ResultMeasureUnit            string             `json:"t_result_measure_unit"`
	ResultReagentsSet            string             `json:"t_result_reagents_set"`

	OtherResults []SQLTestResult    `json:"t_other_results"`
	Histograms   []SQLTestHistogram `json:"t_histograms"`
}

var testsTablesChecked bool = false

func CheckTestsTable() error {

	if testsTablesChecked {
		return nil
	}
	OpenSQLLiteDatabase()

	SQLITEDatabase.CheckTableObj("TESTS", "create table TESTS ("+
		"test_id INTEGER PRIMARY KEY, "+
		"t_order_id int, "+
		"t_name text, "+
		"t_code text, "+
		"t_tag text, "+
		"t_sample_volume int, "+
		"t_selected_for_prog bool, "+
		"t_programmed_in_analyzer_on text, "+
		"t_programmed_in_analyzer_by text, "+
		"tr_result_type int, "+
		"t_curr_result text, "+
		"t_curr_raw_result text, "+
		"t_curr_interpretation text, "+
		"t_curr_result_rcvd_on text, "+
		"t_curr_result_rcvd_by text, "+
		"t_inserted_on text, "+
		"t_inserted_by text, "+
		"t_result_measure_unit text, "+
		"t_result_reagents_set string "+
		")")

	testsTablesChecked = true
	CheckTestResultsTable()
	CheckTestHistogramsTable()
	return nil
}

func (sqltest *SQLTest) ParseFromRawData(rawData map[string]interface{}) error {
	if len(rawData) != 19 {
		return errors.New("RawData not matching the SQLTest type")
	}
	sqltest.Id = rawData["test_id"].(int)
	sqltest.OrderId = rawData["t_order_id"].(int)
	sqltest.Name = rawData["t_name"].(string)
	sqltest.Code = rawData["t_code"].(string)
	sqltest.Tag = rawData["t_tag"].(string)
	sqltest.SampleVolume = rawData["t_sample_volume"].(string)
	sqltest.SelectedToBeProgrammed = rawData["t_selected_for_prog"].(bool)
	sqltest.ProgrammedInAnalyzerDateTime = rawData["t_programmed_in_analyzer_on"].(string)
	sqltest.ProgrammedInAnalyzerBy = rawData["t_programmed_in_analyzer_by"].(string)
	sqltest.ResultType = rawData["tr_result_type"].(SQLOrderResultType)
	sqltest.Result = rawData["t_curr_result"].(string)
	sqltest.Raw = rawData["t_curr_raw_result"].(string)
	sqltest.Interpretation = rawData["t_curr_interpretation"].(string)
	sqltest.ResultReceivedDateTime = rawData["t_curr_result_rcvd_on"].(string)
	sqltest.ResultReceivedBy = rawData["t_curr_result_rcvd_by"].(string)
	sqltest.InsertedDateTime = rawData["t_inserted_on"].(string)
	sqltest.InsertedBy = rawData["t_inserted_by"].(string)
	sqltest.ResultMeasureUnit = rawData["t_result_measure_unit"].(string)
	sqltest.ResultReagentsSet = rawData["t_result_reagents_set"].(string)

	if tmpVal, ok := rawData["t_other_results"]; ok {
		otherResults := []SQLTestResult{}
		otherResultsSON := tmpVal.(string)
		json.Unmarshal([]byte(otherResultsSON), &otherResults)
		sqltest.OtherResults = otherResults
	}

	if tmpVal, ok := rawData["t_histograms"]; ok {
		histograms := []SQLTestHistogram{}
		histogramsJSON := tmpVal.(string)
		json.Unmarshal([]byte(histogramsJSON), &histograms)
		sqltest.Histograms = histograms
	}

	return nil
}

func (sqltest *SQLTest) ParseFromMap(mapData map[string]string) error {
	var err error
	sqltest.Id, err = strconv.Atoi(mapData["test_id"])
	if err != nil {
		sqltest.Id = 0
	}
	sqltest.OrderId, err = strconv.Atoi(mapData["t_order_id"])
	if err != nil {
		sqltest.OrderId = 0
	}
	sqltest.Name = mapData["t_name"]
	sqltest.Code = mapData["t_code"]
	sqltest.Tag = mapData["t_tag"]
	sqltest.SampleVolume = mapData["t_sample_volume"]

	sqltest.SelectedToBeProgrammed, err = strconv.ParseBool(mapData["SelectedToBeProgrammed"])
	if err != nil {
		sqltest.SelectedToBeProgrammed = true
	}
	sqltest.ProgrammedInAnalyzerDateTime = mapData["t_programmed_in_analyzer_on"]
	sqltest.ProgrammedInAnalyzerBy = mapData["t_programmed_in_analyzer_by"]

	tmpInt, err := strconv.Atoi(mapData["tr_result_type"])

	if err != nil {
		sqltest.ResultType = 0
	} else {
		sqltest.ResultType = SQLOrderResultType(tmpInt)
	}

	sqltest.Result = mapData["t_curr_result"]
	sqltest.Raw = mapData["t_curr_raw_result"]
	sqltest.Interpretation = mapData["t_curr_interpretation"]
	sqltest.ResultReceivedDateTime = mapData["t_curr_result_rcvd_on"]
	sqltest.ResultReceivedBy = mapData["t_curr_result_rcvd_by"]
	sqltest.InsertedDateTime = mapData["t_inserted_on"]
	sqltest.InsertedBy = mapData["t_inserted_by"]
	sqltest.ResultMeasureUnit = mapData["t_result_measure_unit"]
	sqltest.ResultReagentsSet = mapData["t_result_reagents_set"]

	if otherResultsSON, ok := mapData["t_other_results"]; ok {
		otherResults := []SQLTestResult{}
		json.Unmarshal([]byte(otherResultsSON), &otherResults)
		sqltest.OtherResults = otherResults
	}

	if histogramsJSON, ok := mapData["t_histograms"]; ok {
		histograms := []SQLTestHistogram{}
		json.Unmarshal([]byte(histogramsJSON), &histograms)
		sqltest.Histograms = histograms
	}

	return nil
}

func (sqltest *SQLTest) ToMap() (map[string]string, error) {
	tmpMap := make(map[string]string)
	tmpMap["test_id"] = strconv.Itoa(sqltest.Id)
	tmpMap["t_order_id"] = strconv.Itoa(sqltest.OrderId)
	tmpMap["t_name"] = sqltest.Name
	tmpMap["t_code"] = sqltest.Code
	tmpMap["t_tag"] = sqltest.Tag
	tmpMap["t_sample_volume"] = sqltest.SampleVolume
	if sqltest.SelectedToBeProgrammed {
		tmpMap["t_selected_for_prog"] = "true"
	} else {
		tmpMap["t_selected_for_prog"] = "false"
	}

	tmpMap["t_programmed_in_analyzer_on"] = sqltest.ProgrammedInAnalyzerDateTime
	tmpMap["t_programmed_in_analyzer_by"] = sqltest.ProgrammedInAnalyzerBy
	tmpMap["tr_result_type"] = strconv.Itoa(int(sqltest.ResultType))
	tmpMap["t_curr_result"] = sqltest.Result
	tmpMap["t_curr_raw_result"] = sqltest.Raw
	tmpMap["t_curr_interpretation"] = sqltest.Interpretation
	tmpMap["t_curr_result_rcvd_on"] = sqltest.ResultReceivedDateTime
	tmpMap["t_curr_result_rcvd_by"] = sqltest.ResultReceivedBy
	tmpMap["t_inserted_on"] = sqltest.InsertedDateTime
	tmpMap["t_inserted_by"] = sqltest.InsertedBy
	tmpMap["t_result_measure_unit"] = sqltest.ResultMeasureUnit
	tmpMap["t_result_reagents_set"] = sqltest.ResultReagentsSet

	jsonTxt, err := json.Marshal(sqltest.OtherResults)
	if err != nil {
		jsonTxt = []byte("[]")
	}
	tmpMap["t_other_results"] = string(jsonTxt)

	jsonTxt, err = json.Marshal(sqltest.Histograms)
	if err != nil {
		jsonTxt = []byte("[]")
	}
	tmpMap["t_histograms"] = string(jsonTxt)

	return tmpMap, nil
}

func GetTestQueryInterface() []interface{} {
	return []interface{}{ // Standard MySQL columns
		new(int),                //test_id
		new(int),                //t_order_id
		new(string),             //t_name
		new(string),             //t_code
		new(string),             //t_tag
		new(string),             //t_sample_volume
		new(bool),               //t_selected_for_prog
		new(string),             //t_programmed_in_analyzer_on
		new(string),             //t_programmed_in_analyzer_by
		new(SQLOrderResultType), //tr_result_type
		new(string),             //t_curr_result
		new(string),             //t_curr_raw_result
		new(string),             //t_curr_interpretation
		new(string),             //t_curr_result_rcvd_on
		new(string),             //t_curr_result_rcvd_by
		new(string),             //t_inserted_on
		new(string),             //t_inserted_by
		new(string),             //t_result_measure_unit
		new(string),             //t_result_reagents_set

		//new(string), //t_other_results
		//new(string), //t_histograms
	}
}

func GetTest(testId int) (SQLTest, error) {
	sqlOrd := SQLTest{}
	if testId > 0 {
		dest := GetTestQueryInterface()
		data, err := SQLITEDatabase.ExecQueryFromTable("TESTS", fmt.Sprintf("select * from TESTS where test_id = %d", testId), dest)
		if err != nil {
			return sqlOrd, err
		}

		if len(data) > 0 {
			err := sqlOrd.ParseFromRawData(data[0])
			if err != nil {
				return SQLTest{}, err
			}
		}
	}

	return sqlOrd, nil
}

type SQLTestQuery struct {
	Id      string `json:"test_id"`
	OrderId string `json:"t_order_id"`
	Tag     string `json:"t_tag"`
	Code    string `json:"t_code"`
}

func GetTests(query SQLTestQuery) ([]SQLTest, error) {
	dest := GetTestQueryInterface()

	dbQuery := "select * from TESTS where (1=1)"
	if query.Id != "" {
		dbQuery += " and test_id='" + query.Id + "'"
	}
	if query.OrderId != "" {
		dbQuery += " and t_order_id='" + query.OrderId + "'"
	}
	if query.Tag != "" {
		dbQuery += " and t_tag='" + query.Tag + "'"
	}
	if query.Code != "" {
		dbQuery += " and t_code='" + query.Code + "'"
	}

	data, err := SQLITEDatabase.ExecQueryFromTable("TESTS", dbQuery, dest)
	if err != nil {
		return nil, err
	}

	sqlTests := []SQLTest{}
	for i := range data {
		sqlTest := SQLTest{}
		err := sqlTest.ParseFromRawData(data[i])
		if err != nil {
			return []SQLTest{}, err
		}
		sqlTests = append(sqlTests, sqlTest)
	}

	return sqlTests, nil
}

func transformResult(kt SQLKnownTest, raw string, rawInterp string) (string, string, error) {
	if debugSQLWrapperLevel > 10 {
		fmt.Println("Calling transformResult")
	}
	var res = raw
	switch kt.ResultType {
	case KATypeQuantitative:
		res64, err := strconv.ParseFloat(raw, 32)
		if err != nil {
			res64 = 0.0
		}
		res32 := float32(res64)
		if kt.ResultWeighting != 0.0 {
			res32 = res32 * kt.ResultWeighting
		}

		resFmt := int(kt.ResultFormatting) - 2
		if resFmt >= 0 {
			res = fmt.Sprintf("%."+strconv.Itoa(resFmt)+"f", res32)
		} else {
			res = fmt.Sprintf("%f", res32)
		}
		break
	default:
		break
	}
	res = strings.Trim(res, " ")

	if debugSQLWrapperLevel > 10 {
		fmt.Printf("\nApplying transfomations pairs on %s:\n%q", res, kt.ResultTransformation)
	}
	for _, trans := range kt.ResultTransformation {
		if res == trans.From {
			res = trans.To
		}
	}

	return res, strings.Trim(rawInterp, " "), nil
}
func getKnownTest(code string, tag string) (*SQLKnownTest, error) {
	ktQuery := SQLKnownTestQuery{}
	if code != "" {
		ktQuery.Code = code
	}
	if tag != "" {
		ktQuery.Tag = tag
	}

	kts, err := GetKnownTestsQuery(ktQuery)
	if err != nil {
		return nil, err
	}

	if len(kts) <= 0 {
		return nil, errors.New(fmt.Sprintf("Unknown test code %s - tag %s", code, tag))
	}

	return &kts[0], nil
}

func verifyTestData(testData *SQLTest) error {
	//verify if the testData is legit
	kt, err := getKnownTest(testData.Code, testData.Tag)
	if err != nil {
		return err
	}
	if testData.Raw != "" {
		testData.Result, testData.Interpretation, err = transformResult(*kt, testData.Raw, testData.Interpretation)
		if err != nil {
			return err
		}
	} else {
		testData.Result = ""
		testData.Interpretation = ""
	}

	testData.ResultMeasureUnit = kt.ResultMeasureUnit
	testData.ResultReagentsSet = kt.ResultReagentsSet
	return nil
}

func SaveTest(cond SQLTestQuery, testData SQLTest) (bool, int, error) {
	CheckTestsTable()

	err := verifyTestData(&testData)
	if err != nil {
		return false, 0, err
	}

	tests, err := GetTests(cond)
	if err != nil {
		return false, 0, err
	}

	var execParams []interface{}
	if len(tests) > 0 {
		for _, test := range tests {
			if debugSQLWrapperLevel > 10 {
				fmt.Printf("\nUpdating Test \n%q\n--------------------------------------\n", testData)
			}
			stmt, err := SQLITEDatabase.DbObj.Prepare("update TESTS set t_order_id = ?,t_name=?,t_code=?,t_tag=?,t_sample_volume=?,t_selected_for_prog=?,t_programmed_in_analyzer_on=?,t_programmed_in_analyzer_by=?,tr_result_type=?,t_curr_result=?,t_curr_raw_result=?,t_curr_interpretation=?,t_curr_result_rcvd_on=?,t_curr_result_rcvd_by=?,t_inserted_on=?,t_inserted_by=?,t_result_measure_unit=?,t_result_reagents_set=?  where test_id = ?")

			if err != nil {
				return false, 0, err
			}
			execParams = []interface{}{testData.OrderId, testData.Name, testData.Code, testData.Tag, testData.SampleVolume, testData.SelectedToBeProgrammed, testData.ProgrammedInAnalyzerDateTime, testData.ProgrammedInAnalyzerBy, testData.ResultType, testData.Result, testData.Raw, testData.Interpretation, testData.ResultReceivedDateTime, testData.ResultReceivedBy, testData.InsertedDateTime, testData.InsertedBy, testData.ResultMeasureUnit, testData.ResultReagentsSet, test.Id}

			_, err = stmt.Exec(execParams...)
			if err != nil {
				return false, 0, err
			}
			stmt.Close()
		}
		return false, 0, nil
	} else {
		//insert
		if debugSQLWrapperLevel > 10 {
			fmt.Printf("\nInserting Test \n%q\n--------------------------------------\n", testData)
		}

		if testData.Code == "" || testData.Tag == "" || testData.Name == "" {
			knownTest, err := getKnownTest(testData.Code, testData.Tag)
			if err != nil {
				return false, 0, err
			}
			if testData.Code == "" {
				testData.Code = knownTest.Code
			}
			if testData.Tag == "" {
				testData.Tag = knownTest.Tag
			}
			if testData.Name == "" {
				testData.Name = knownTest.Code
			}
		}

		if general.LoggedInUser != nil {
			testData.InsertedBy = string(general.LoggedInUser.UserId)
		}
		nowt := time.Now()
		testData.InsertedDateTime = nowt.Format("2006-01-02 15:04:05")

		stmt, err := SQLITEDatabase.DbObj.Prepare("insert into TESTS(t_order_id,t_name,t_code,t_tag,t_sample_volume,t_selected_for_prog,t_programmed_in_analyzer_on,t_programmed_in_analyzer_by,tr_result_type,t_curr_result,t_curr_raw_result,t_curr_interpretation,t_curr_result_rcvd_on,t_curr_result_rcvd_by, t_inserted_on, t_inserted_by, t_result_measure_unit, t_result_reagents_set) values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		if err != nil {
			return false, 0, err
		}
		execParams = []interface{}{testData.OrderId, testData.Name, testData.Code, testData.Tag, testData.SampleVolume, testData.SelectedToBeProgrammed, testData.ProgrammedInAnalyzerDateTime, testData.ProgrammedInAnalyzerBy, testData.ResultType, testData.Result, testData.Raw, testData.Interpretation, testData.ResultReceivedDateTime, testData.ResultReceivedBy, testData.InsertedDateTime, testData.InsertedBy, testData.ResultMeasureUnit, testData.ResultReagentsSet}
		resp, err := stmt.Exec(execParams...)
		if err != nil {
			return false, 0, err
		}
		defer stmt.Close()

		lastInsertId, err := resp.LastInsertId()
		if err != nil {
			return false, 0, err
		}

		return true, int(lastInsertId), nil
	}
}

func SaveTestsBulk(queue *general.ObjectQueue) {
	for queue.Len() > 0 {
		tmpObj, err := queue.Pop()
		if err != nil {
			fmt.Printf("Erroron save result: %v\n", err)
		}
		tmpTest, ok := tmpObj.(SQLTest)
		if ok {
			if debugSQLWrapperLevel > 10 {
				fmt.Println("New file result to save %v\n", tmpTest)
			}
			SaveTest(SQLTestQuery{Id: strconv.Itoa(tmpTest.Id)}, tmpTest)
		}
	}
}

func SaveTestFromMap(testId int, testData map[string]string) (bool, int, error) {
	testObj := SQLTest{}
	testObj.ParseFromMap(testData)
	return SaveTest(SQLTestQuery{Id: strconv.Itoa(testId)}, testObj)
}

func DeleteTest(testId int) error {
	if debugSQLWrapperLevel > 10 {
		fmt.Println("Deleting test")
	}

	tstsRes, err := GetTestResults(SQLTestResultQuery{TestId: strconv.Itoa(testId)})
	if err != nil {
		fmt.Printf("\n Cannot get test results for test %d \n", testId)
		return err
	}
	for _, tstRes := range tstsRes {
		DeleteTestResult(tstRes.Id)
	}

	stmt, err := SQLITEDatabase.DbObj.Prepare("delete from TESTS where test_id = ?")
	defer stmt.Close()

	if err != nil {
		return err
	}
	execParams := []interface{}{testId}

	_, err = stmt.Exec(execParams...)
	if err != nil {
		return err
	}
	return nil
}
