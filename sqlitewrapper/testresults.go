package sqlitewrapper

import (
	"fmt"
	"github.com/pkg/errors"
	"strconv"
	"wisemed-labreaders/general"
)

type SQLTestResult struct {
	Id                     int    `json:"test_result_id"`
	TestId                 int    `json:"tr_test_id"`
	Result                 string `json:"tr_result"`
	Raw                    string `json:"tr_raw_result"`
	Interpretation         string `json:"tr_interpretation"`
	ResultReceivedDateTime string `json:"tr_result_received_on"`
	ResultReceivedBy       string `json:"tr_result_received_by"`
}

var testResultsTablesChecked bool = false

func CheckTestResultsTable() error {
	if testResultsTablesChecked {
		return nil
	}
	OpenSQLLiteDatabase()

	SQLITEDatabase.CheckTableObj("TESTS_RESULTS", "create table TESTS_RESULTS ("+
		"test_result_id INTEGER PRIMARY KEY, "+
		"tr_test_id int, "+
		"tr_result text, "+
		"tr_raw_result text, "+
		"tr_interpretation text, "+
		"tr_rcvd_on datetime, "+
		"tr_rcvd_by string "+
		")")

	testResultsTablesChecked = true
	return nil
}

func (sqltestresut *SQLTestResult) ParseFromRawData(rawData map[string]interface{}) error {
	if len(rawData) != 6 {
		return errors.New("RawData not matching the SQLTestResult type")
	}
	sqltestresut.Id = rawData["test_result_id"].(int)
	sqltestresut.TestId = rawData["tr_test_id"].(int)
	sqltestresut.Result = rawData["tr_result"].(string)
	sqltestresut.Raw = rawData["tr_raw_result"].(string)
	sqltestresut.Interpretation = rawData["tr_interpretation"].(string)
	sqltestresut.ResultReceivedDateTime = rawData["tr_result_received_on"].(string)
	sqltestresut.ResultReceivedBy = rawData["tr_result_received_by"].(string)

	return nil
}

func (sqltestresut *SQLTestResult) ParseFromMap(mapData map[string]string) error {
	var err error
	sqltestresut.Id, err = strconv.Atoi(mapData["test_result_id"])
	if err != nil {
		sqltestresut.Id = 0
	}
	sqltestresut.TestId, err = strconv.Atoi(mapData["tr_test_id"])
	sqltestresut.Result = mapData["tr_result"]
	sqltestresut.Raw = mapData["tr_raw_result"]
	sqltestresut.Interpretation = mapData["tr_interpretation"]
	sqltestresut.ResultReceivedDateTime = mapData["tr_result_received_on"]
	sqltestresut.ResultReceivedBy = mapData["tr_result_received_by"]

	return nil
}

func (sqltestresut *SQLTestResult) ToMap() (map[string]string, error) {
	tmpMap := make(map[string]string)
	tmpMap["test_result_id"] = strconv.Itoa(sqltestresut.Id)
	tmpMap["tr_test_id"] = strconv.Itoa(sqltestresut.TestId)
	tmpMap["tr_result"] = sqltestresut.Result
	tmpMap["tr_raw_result"] = sqltestresut.Raw
	tmpMap["tr_interpretation"] = sqltestresut.Interpretation
	tmpMap["tr_result_received_on"] = sqltestresut.ResultReceivedDateTime
	tmpMap["tr_result_received_by"] = sqltestresut.ResultReceivedBy

	return tmpMap, nil
}

func GetTestResultQueryInterface() []interface{} {
	return []interface{}{ // Standard MySQL columns
		new(int),    //test_result_id
		new(string), //tr_result
		new(string), //tr_raw_result
		new(string), //tr_interpretation
		new(string), //tr_result_received_on
		new(string), //tr_result_received_by
	}
}

func GetTestResult(testResultId int) (SQLTestResult, error) {
	sqlOrd := SQLTestResult{}
	if testResultId > 0 {
		dest := GetTestResultQueryInterface()
		data, err := SQLITEDatabase.ExecQueryFromTable("TESTS_RESULTS", fmt.Sprintf("select * from TESTS_RESULTS where test_result_id = %d", testResultId), dest)
		if err != nil {
			return sqlOrd, err
		}

		if len(data) > 0 {
			err := sqlOrd.ParseFromRawData(data[0])
			if err != nil {
				return SQLTestResult{}, err
			}
		}
	}

	return sqlOrd, nil
}

type SQLTestResultQuery struct {
	Id     string `json:"test_result_id"`
	TestId string `json:"tr_test_id"`
}

func GetTestResults(query SQLTestResultQuery) ([]SQLTestResult, error) {
	dest := GetTestResultQueryInterface()

	dbQuery := "select * from TESTS_RESULTS where (1=1)"
	if query.Id != "" {
		dbQuery += " and test_result_id='" + query.Id + "'"
	}
	if query.TestId != "" {
		dbQuery += " and tr_test_id='" + query.TestId + "'"
	}

	data, err := SQLITEDatabase.ExecQueryFromTable("TESTS_RESULTS", dbQuery, dest)
	if err != nil {
		return nil, err
	}

	sqlTestResults := []SQLTestResult{}
	for i := range data {
		sqlTestResult := SQLTestResult{}
		err := sqlTestResult.ParseFromRawData(data[i])
		if err != nil {
			return []SQLTestResult{}, err
		}
		sqlTestResults = append(sqlTestResults, sqlTestResult)
	}

	return sqlTestResults, nil
}

func SaveTestResult(testResultId int, trData SQLTestResult) error {
	stmt, err := SQLITEDatabase.DbObj.Prepare("select * from TESTS_RESULTS where test_result_id = ?")
	if err != nil {
		return err
	}

	rows, err := stmt.Query(testResultId)
	if err != nil {
		return err
	}
	stmt.Close()
	if debugSQLWrapperLevel > 10 {
		fmt.Print(rows)
	}

	var execParams []interface{}
	if rows.Next() {
		//update
		if debugSQLWrapperLevel > 10 {
			fmt.Println("Updating test result")
		}
		stmt, err = SQLITEDatabase.DbObj.Prepare("update TESTS_RESULTS set tr_test_id = ?,tr_result=?,tr_raw_result=?,tr_interpretation=?,tr_result_received_on=?,tr_result_received_by=?  where test_result_id = ?")

		if err != nil {
			return err
		}
		execParams = []interface{}{trData.TestId, trData.Result, trData.Raw, trData.Interpretation, trData.ResultReceivedDateTime, trData.ResultReceivedBy, testResultId}

	} else {
		//insert
		if debugSQLWrapperLevel > 10 {
			fmt.Println("Inserting test result")
		}
		stmt, err = SQLITEDatabase.DbObj.Prepare("insert into TESTS_RESULTS(tr_test_id, tr_result, tr_raw_result,tr_interpretation,tr_result_received_on,tr_result_received_by) values(?, ?, ?, ?, ?, ?)")
		if err != nil {
			return err
		}
		execParams = []interface{}{trData.TestId, trData.Result, trData.Raw, trData.Interpretation, trData.ResultReceivedDateTime, trData.ResultReceivedBy}
	}
	rows.Close()

	_, err = stmt.Exec(execParams...)
	if err != nil {
		return err
	}
	defer stmt.Close()

	return nil
}

func SaveTestResultsBulk(queue *general.ObjectQueue) {
	for queue.Len() > 0 {
		tmpObj, err := queue.Pop()
		if err != nil {
			fmt.Printf("Erroron save result: %v\n", err)
		}
		tmpTR, ok := tmpObj.(SQLTestResult)
		if ok {
			if debugSQLWrapperLevel > 10 {
				fmt.Println("New test result to save %v\n", tmpTR)
			}
			SaveTestResult(tmpTR.Id, tmpTR)
		}
	}
}

func SaveTestResultFromMap(testResultId int, trData map[string]string) error {
	ktObj := SQLTestResult{}
	ktObj.ParseFromMap(trData)
	return SaveTestResult(testResultId, ktObj)
}

func DeleteTestResult(testResultId int) error {
	if debugSQLWrapperLevel > 10 {
		fmt.Println("Deleting test result")
	}
	stmt, err := SQLITEDatabase.DbObj.Prepare("delete from TESTS_RESULTS where test_result_id = ?")
	defer stmt.Close()

	if err != nil {
		return err
	}
	execParams := []interface{}{testResultId}

	_, err = stmt.Exec(execParams...)
	if err != nil {
		return err
	}
	return nil
}
