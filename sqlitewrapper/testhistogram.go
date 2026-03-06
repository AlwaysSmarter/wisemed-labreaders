package sqlitewrapper

import (
	"fmt"
	"github.com/pkg/errors"
	"strconv"
	"wisemed-labreaders/general"
)

type SQLTestHistogram struct {
	Id          int    `json:"test_histogram_id"`
	TestId      int    `json:"hst_test_id"`
	Name        string `json:"hst_name"`
	GraphPoints string `json:"hst_graph_points"`
	MinPoints   string `json:"hst_min_points"` //MinPoints
}

var testHistogramsTablesChecked bool = false

func CheckTestHistogramsTable() error {
	if testHistogramsTablesChecked {
		return nil
	}
	OpenSQLLiteDatabase()

	SQLITEDatabase.CheckTableObj("TESTS_HISTOGRAMS", "create table TESTS_HISTOGRAMS ("+
		"test_histogram_id INTEGER PRIMARY KEY, "+
		"hst_test_id int, "+
		"hst_name text, "+
		"hst_graph_points text "+
		")")

	testHistogramsTablesChecked = true
	return nil
}

func (sqltestresut *SQLTestHistogram) ParseFromRawData(rawData map[string]interface{}) error {
	if len(rawData) != 10 {
		return errors.New("RawData not matching the SQLTestHistogram type")
	}
	sqltestresut.Id = rawData["test_histogram_id"].(int)
	sqltestresut.TestId = rawData["hst_test_id"].(int)
	sqltestresut.Name = rawData["hst_name"].(string)
	sqltestresut.GraphPoints = rawData["hst_graph_points"].(string)
	sqltestresut.MinPoints = rawData["hst_min_points"].(string)

	return nil
}

func (sqltestresut *SQLTestHistogram) ParseFromMap(mapData map[string]string) error {
	var err error
	sqltestresut.Id, err = strconv.Atoi(mapData["test_histogram_id"])
	if err != nil {
		sqltestresut.Id = 0
	}
	sqltestresut.TestId, err = strconv.Atoi(mapData["hst_test_id"])
	sqltestresut.Name = mapData["hst_name"]
	sqltestresut.GraphPoints = mapData["hst_graph_points"]
	sqltestresut.MinPoints = mapData["hst_min_points"]

	return nil
}

func (sqltestresut *SQLTestHistogram) ToMap() (map[string]string, error) {
	tmpMap := make(map[string]string)
	tmpMap["test_histogram_id"] = strconv.Itoa(sqltestresut.Id)
	tmpMap["hst_test_id"] = strconv.Itoa(sqltestresut.TestId)
	tmpMap["hst_name"] = sqltestresut.Name
	tmpMap["hst_graph_points"] = sqltestresut.GraphPoints
	tmpMap["hst_min_points"] = sqltestresut.MinPoints

	return tmpMap, nil
}

func GetTestHistogramQueryInterface() []interface{} {
	return []interface{}{ // Standard MySQL columns
		new(int),    //test_histogram_id
		new(string), //hst_name
		new(string), //hst_graph_points
		new(string), //hst_min_points
		new(string), //hst_name_received_on
		new(string), //hst_name_received_by
	}
}

func GetTestHistogram(testHistogramId int) (SQLTestHistogram, error) {
	sqlOrd := SQLTestHistogram{}
	if testHistogramId > 0 {
		dest := GetTestHistogramQueryInterface()
		data, err := SQLITEDatabase.ExecQueryFromTable("TESTS_HISTOGRAMS", fmt.Sprintf("select * from TESTS_HISTOGRAMS where test_histogram_id = %d", testHistogramId), dest)
		if err != nil {
			return sqlOrd, err
		}

		if len(data) > 0 {
			err := sqlOrd.ParseFromRawData(data[0])
			if err != nil {
				return SQLTestHistogram{}, err
			}
		}
	}

	return sqlOrd, nil
}

type SQLTestHistogramQuery struct {
	Id     string `json:"test_histogram_id"`
	TestId string `json:"hst_test_id"`
}

func GetTestHistograms(query SQLTestHistogramQuery) ([]SQLTestHistogram, error) {
	dest := GetTestHistogramQueryInterface()

	dbQuery := "select * from TESTS_HISTOGRAMS where (1=1)"
	if query.Id != "" {
		dbQuery += " and test_histogram_id='" + query.Id + "'"
	}
	if query.TestId != "" {
		dbQuery += " and hst_test_id='" + query.TestId + "'"
	}

	data, err := SQLITEDatabase.ExecQueryFromTable("TESTS_HISTOGRAMS", dbQuery, dest)
	if err != nil {
		return nil, err
	}

	sqlTestHistograms := []SQLTestHistogram{}
	for i := range data {
		sqlTestHistogram := SQLTestHistogram{}
		err := sqlTestHistogram.ParseFromRawData(data[i])
		if err != nil {
			return []SQLTestHistogram{}, err
		}
		sqlTestHistograms = append(sqlTestHistograms, sqlTestHistogram)
	}

	return sqlTestHistograms, nil
}

func SaveTestHistogram(testHistogramId int, trData SQLTestHistogram) error {
	stmt, err := SQLITEDatabase.DbObj.Prepare("select * from TESTS_HISTOGRAMS where test_histogram_id = ?")
	if err != nil {
		return err
	}

	rows, err := stmt.Query(testHistogramId)
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
		stmt, err = SQLITEDatabase.DbObj.Prepare("update TESTS_HISTOGRAMS set hst_test_id = ?,hst_name=?,hst_graph_points=?,hst_min_points=?  where test_histogram_id = ?")

		if err != nil {
			return err
		}
		execParams = []interface{}{trData.TestId, trData.Name, trData.GraphPoints, trData.MinPoints}

	} else {
		//insert
		if debugSQLWrapperLevel > 10 {
			fmt.Println("Inserting test result")
		}
		stmt, err = SQLITEDatabase.DbObj.Prepare("insert into TESTS_HISTOGRAMS(hst_test_id, hst_name, hst_graph_points,hst_min_points) values(?, ?, ?, ?)")
		if err != nil {
			return err
		}
		execParams = []interface{}{trData.TestId, trData.Name, trData.GraphPoints, trData.MinPoints}
	}
	rows.Close()

	_, err = stmt.Exec(execParams...)
	if err != nil {
		return err
	}
	defer stmt.Close()

	return nil
}

func SaveTestHistogramsBulk(queue *general.ObjectQueue) {
	for queue.Len() > 0 {
		tmpObj, err := queue.Pop()
		if err != nil {
			fmt.Printf("Erroron save result: %v\n", err)
		}
		tmpTH, ok := tmpObj.(SQLTestHistogram)
		if ok {
			if debugSQLWrapperLevel > 10 {
				fmt.Println("New test histogram to save %v\n", tmpTH)
			}
			SaveTestHistogram(tmpTH.Id, tmpTH)
		}
	}
}

func SaveTestHistogramFromMap(testHistogramId int, trData map[string]string) error {
	ktObj := SQLTestHistogram{}
	ktObj.ParseFromMap(trData)
	return SaveTestHistogram(testHistogramId, ktObj)
}

func DeleteTestHistogram(testHistogramId int) error {
	if debugSQLWrapperLevel > 10 {
		fmt.Println("Deleting test histogram")
	}
	stmt, err := SQLITEDatabase.DbObj.Prepare("delete from TESTS_HISTOGRAMS where test_histogram_id = ?")
	defer stmt.Close()

	if err != nil {
		return err
	}
	execParams := []interface{}{testHistogramId}

	_, err = stmt.Exec(execParams...)
	if err != nil {
		return err
	}
	return nil
}
