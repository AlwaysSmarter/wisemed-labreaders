package sqlitewrapper

import (
	"fmt"
	"github.com/pkg/errors"
	"strconv"
	"wisemed-labreaders/general"
)

type SQLTestHistory struct {
	Id     int    `json:"test_history_id"`
	TestId int    `json:"th_test_id"`
	SentOn string `json:"th_sent_on"`
	SentBy string `json:"th_sent_by"`
	Status string `json:"th_status"` //S - sent C - confirmed by analyzer E - error
}

var testHistoryTablesChecked bool = false

func SaveHistoryToDatabase(queue *general.ObjectQueue) {
	for queue.Len() > 0 {
		tmpObj, err := queue.Pop()
		if err != nil {
			fmt.Printf("Erroron save history: %v\n", err)
		}
		tmpPac, ok := tmpObj.(SQLTestHistory)
		if ok {
			if debugSQLWrapperLevel > 10 {
				fmt.Println("New file history to save %v\n", tmpPac)
			}
		}
	}
}
func CheckTestHistoryTable() error {
	if testHistoryTablesChecked {
		return nil
	}
	OpenSQLLiteDatabase()

	SQLITEDatabase.CheckTableObj("TESTS_HISTORY", "create table TESTS_HISTORY ("+
		"test_history_id INTEGER PRIMARY KEY, "+
		"th_test_id int, "+
		"th_sent_on text, "+
		"th_sent_by text, "+
		"th_status text "+
		")")

	testHistoryTablesChecked = true
	return nil
}

func (sqltestresut *SQLTestHistory) ParseFromRawData(rawData map[string]interface{}) error {
	if len(rawData) != 5 {
		return errors.New("RawData not matching the SQLTestHistory type")
	}
	sqltestresut.Id = rawData["test_history_id"].(int)
	sqltestresut.TestId = rawData["th_test_id"].(int)
	sqltestresut.SentOn = rawData["th_sent_on"].(string)
	sqltestresut.SentBy = rawData["th_sent_by"].(string)
	sqltestresut.Status = rawData["th_status"].(string)

	return nil
}

func (sqltestresut *SQLTestHistory) ParseFromMap(mapData map[string]string) error {
	var err error
	sqltestresut.Id, err = strconv.Atoi(mapData["test_history_id"])
	if err != nil {
		sqltestresut.Id = 0
	}
	sqltestresut.TestId, err = strconv.Atoi(mapData["th_test_id"])
	sqltestresut.SentOn = mapData["th_sent_on"]
	sqltestresut.SentBy = mapData["th_sent_by"]
	sqltestresut.Status = mapData["th_status"]

	return nil
}

func (sqltestresut *SQLTestHistory) ToMap() (map[string]string, error) {
	tmpMap := make(map[string]string)
	tmpMap["test_history_id"] = strconv.Itoa(sqltestresut.Id)
	tmpMap["th_test_id"] = strconv.Itoa(sqltestresut.TestId)
	tmpMap["th_sent_on"] = sqltestresut.SentOn
	tmpMap["th_sent_by"] = sqltestresut.SentBy
	tmpMap["th_status"] = sqltestresut.Status

	return tmpMap, nil
}

func GetTestHistoryQueryInterface() []interface{} {
	return []interface{}{ // Standard MySQL columns
		new(int),    //test_history_id
		new(int),    //th_test_id
		new(string), //th_sent_on
		new(string), //th_sent_by
		new(string), //th_status
	}
}

func GetTestHistory(testHistoryId int) (SQLTestHistory, error) {
	CheckTestHistoryTable()
	sqlOrd := SQLTestHistory{}
	if testHistoryId > 0 {
		dest := GetTestHistoryQueryInterface()
		data, err := SQLITEDatabase.ExecQueryFromTable("TESTS_HISTORY", fmt.Sprintf("select * from TESTS_HISTORY where test_history_id = %d", testHistoryId), dest)
		if err != nil {
			return sqlOrd, err
		}

		if len(data) > 0 {
			err := sqlOrd.ParseFromRawData(data[0])
			if err != nil {
				return SQLTestHistory{}, err
			}
		}
	}

	return sqlOrd, nil
}

type SQLTestHistoryQuery struct {
	Id     string `json:"test_history_id"`
	TestId string `json:"th_test_id"`
}

func GetTestHistoryAll(query SQLTestHistoryQuery) ([]SQLTestHistory, error) {
	CheckTestHistoryTable()
	dest := GetTestHistoryQueryInterface()

	dbQuery := "select * from TESTS_HISTORY where (1=1)"
	if query.Id != "" {
		dbQuery += " and test_history_id='" + query.Id + "'"
	}
	if query.TestId != "" {
		dbQuery += " and th_test_id='" + query.TestId + "'"
	}

	data, err := SQLITEDatabase.ExecQueryFromTable("TESTS_HISTORY", dbQuery, dest)
	if err != nil {
		return nil, err
	}

	sqlTestHistoryAll := []SQLTestHistory{}
	for i := range data {
		sqlTestHistory := SQLTestHistory{}
		err := sqlTestHistory.ParseFromRawData(data[i])
		if err != nil {
			return []SQLTestHistory{}, err
		}
		sqlTestHistoryAll = append(sqlTestHistoryAll, sqlTestHistory)
	}

	return sqlTestHistoryAll, nil
}

func SaveTestHistory(cond SQLTestHistoryQuery, trData SQLTestHistory) (bool, int, error) {
	CheckOrdersTable()
	testshistory, err := GetTestHistoryAll(cond)
	if err != nil {
		return false, 0, err
	}

	var execParams []interface{}
	if len(testshistory) > 0 {
		for _, testh := range testshistory {
			if debugSQLWrapperLevel > 10 {
				fmt.Println("Updating test history")
			}
			stmt, err := SQLITEDatabase.DbObj.Prepare("update TESTS_HISTORY set th_test_id = ?,th_sent_on=?,th_sent_by=?,th_status=?  where test_history_id = ?")

			if err != nil {
				return false, 0, err
			}
			execParams = []interface{}{trData.TestId, trData.SentOn, trData.SentBy, trData.Status, testh.Id}

			_, err = stmt.Exec(execParams...)
			if err != nil {
				return false, 0, err
			}
			stmt.Close()
		}
		return false, 0, nil
	} else {
		if debugSQLWrapperLevel > 10 {
			fmt.Println("Inserting test history")
		}
		stmt, err := SQLITEDatabase.DbObj.Prepare("insert into TESTS_HISTORY(th_test_id, th_sent_on, th_sent_by, th_status) values(?, ?, ?, ?)")
		defer stmt.Close()
		if err != nil {
			fmt.Println("ERR -1", err)
			return false, 0, err
		}

		execParams = []interface{}{trData.TestId, trData.SentOn, trData.SentBy, trData.Status}

		resp, err := stmt.Exec(execParams...)
		if err != nil {
			fmt.Println("ERR 0", err)
			return false, 0, err
		}

		lastInsertId, err := resp.LastInsertId()
		if err != nil {
			fmt.Println("ERR 1", err)
			return false, 0, err
		}

		return true, int(lastInsertId), nil
	}
}

func SaveTestHistoryBulk(queue *general.ObjectQueue) {
	CheckTestHistoryTable()
	for queue.Len() > 0 {
		tmpObj, err := queue.Pop()
		if err != nil {
			fmt.Printf("Erroron save history: %v\n", err)
		}
		tmpTR, ok := tmpObj.(SQLTestHistory)
		if ok {
			if debugSQLWrapperLevel > 10 {
				fmt.Println("New test history to save %v\n", tmpTR)
			}
			thq := SQLTestHistoryQuery{Id: strconv.Itoa(tmpTR.Id)}
			SaveTestHistory(thq, tmpTR)
		}
	}
}

func SaveTestHistoryFromMap(testHistoryId int, trData map[string]string) error {
	ktObj := SQLTestHistory{}
	ktObj.ParseFromMap(trData)
	thq := SQLTestHistoryQuery{Id: strconv.Itoa(testHistoryId)}
	_, _, err := SaveTestHistory(thq, ktObj)
	return err
}

func DeleteTestHistory(testHistoryId int) error {
	CheckTestHistoryTable()
	if debugSQLWrapperLevel > 10 {
		fmt.Println("Deleting test history")
	}
	stmt, err := SQLITEDatabase.DbObj.Prepare("delete from TESTS_HISTORY where test_history_id = ?")
	defer stmt.Close()

	if err != nil {
		return err
	}
	execParams := []interface{}{testHistoryId}

	_, err = stmt.Exec(execParams...)
	if err != nil {
		return err
	}
	return nil
}
