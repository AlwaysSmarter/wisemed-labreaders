package sqlitewrapper

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"strconv"
	"time"
	"wisemed-labreaders/general"
)

type SQLOrderResultType int

const (
	ResultPatient SQLOrderResultType = iota
	ResultQC
)

type SQLOrderQuery struct {
	Id         string `json:"order_id"`
	OnDate     string `json:"on_date"`
	FromDate   string `json:"from_date"`
	ToDate     string `json:"to_date"`
	Roundno    string `json:"round_no"`
	RackNo     string `json:"rack_no"`
	PositionNo string `json:"position_no"`
	FileId     string `json:"file_id"`
	PatientId  string `json:"patient_id"`
}

type SQLOrder struct {
	Id                      int    `json:"order_id"`
	Date                    string `json:"o_date"`
	PatientId               string `json:"o_patient_id"`
	FileId                  string `json:"o_file_id"`
	FileCode                string `json:"o_file_code"`
	FileCode2               string `json:"o_file_code2"`
	FileDate                string `json:"o_file_date"`
	RoundNo                 int    `json:"o_round_no"`
	RackNo                  int    `json:"o_rack_no"`
	PositionNo              int    `json:"o_position_no"`
	SampleNo                int    `json:"o_sample_no"`
	PatientName             string `json:"o_patient_name"`
	PatientSex              string
	PatientBirthdate        string
	ResultType              SQLOrderResultType `json:"o_result_type"`
	QCLevelInfo             string             `json:"o_qc_level_info"`
	QCLotInfo               string             `json:"o_qc_lt_info"`
	OrderProgrammedDateTime string             `json:"o_programmed_on"`
	OrderProgrammedBy       string             `json:"o_programmed_by"`
	ResultReceivedDateTime  string             `json:"o_result_received_on"`
	ResultReceivedBy        string             `json:"o_result_received_by"`
	ResultConfimedDateTime  string             `json:"o_result_confirmed_on"`
	ResultConfimedBy        string             `json:"o_result_confirmed_by"`
	Tests                   []SQLTest          `json:"o_tests"`
}

type OrderStats struct {
	ProgrammedPatients      int
	FinalizedPatients       int
	AnalisysInWork          int
	WorkingCapabilityTotal  int
	WorkingCapabilityActive int
}

var ordersTablesChecked bool = false

func CheckOrdersTable() error {
	if ordersTablesChecked {
		return nil
	}
	OpenSQLLiteDatabase()

	SQLITEDatabase.CheckTableObj("ORDERS", "create table ORDERS ("+
		"order_id INTEGER PRIMARY KEY, "+
		"o_date date, "+
		"o_patient_id text, "+
		"o_file_id text, "+
		"o_file_code text, "+
		"o_file_code2 text, "+
		"o_file_date text, "+
		"o_round_no int, "+
		"o_rack_no int, "+
		"o_position_no int, "+
		"o_sample_no int, "+
		"o_patient_name text, "+
		"o_result_type text, "+
		"o_qc_level_info text, "+
		"o_qc_lt_info text, "+
		"o_programmed_on text, "+
		"o_programmed_by text, "+
		"o_result_received_on text, "+
		"o_result_received_by text, "+
		"o_result_confirmed_on text, "+
		"o_result_confirmed_by text "+
		")")
	CheckTestsTable()

	ordersTablesChecked = true
	return nil
}

func (sqlord *SQLOrder) ParseFromRawData(rawData map[string]interface{}, formatDateForWeb bool) error {
	if len(rawData) != 21 {
		return errors.New("RawData not matching the SQLOrder type")
	}
	sqlord.Id = rawData["order_id"].(int)
	sqlord.Date = rawData["o_date"].(string)
	sqlord.PatientId = fmt.Sprintf("%v", rawData["o_patient_id"])
	sqlord.FileId = fmt.Sprintf("%v", rawData["o_file_id"])
	sqlord.FileCode = rawData["o_file_code"].(string)
	sqlord.FileCode2 = rawData["o_file_code2"].(string)
	sqlord.FileDate = rawData["o_file_date"].(string)
	sqlord.RoundNo = rawData["o_round_no"].(int)
	sqlord.RackNo = rawData["o_rack_no"].(int)
	sqlord.PositionNo = rawData["o_position_no"].(int)
	sqlord.SampleNo = rawData["o_sample_no"].(int)
	sqlord.PatientName = rawData["o_patient_name"].(string)
	sqlord.ResultType = rawData["o_result_type"].(SQLOrderResultType)
	sqlord.QCLevelInfo = rawData["o_qc_level_info"].(string)
	sqlord.QCLotInfo = rawData["o_qc_lt_info"].(string)
	sqlord.OrderProgrammedDateTime = rawData["o_programmed_on"].(string)
	sqlord.OrderProgrammedBy = rawData["o_programmed_by"].(string)
	sqlord.ResultReceivedDateTime = rawData["o_result_received_on"].(string)
	sqlord.ResultReceivedBy = rawData["o_result_received_by"].(string)
	sqlord.ResultConfimedDateTime = rawData["o_result_confirmed_on"].(string)
	sqlord.ResultConfimedBy = rawData["o_result_confirmed_by"].(string)
	if formatDateForWeb {
		sqlord.FormatDatesForWeb()
	}
	if tmpVal, ok := rawData["o_tests"]; ok {
		tests := []SQLTest{}
		testJSON := tmpVal.(string)
		json.Unmarshal([]byte(testJSON), &tests)
		sqlord.Tests = tests
	}

	return nil
}

func (sqlord *SQLOrder) ParseFromMap(mapData map[string]string) error {
	var err error
	sqlord.Id, err = strconv.Atoi(mapData["order_id"])
	if err != nil {
		sqlord.Id = 0
	}
	sqlord.Date = mapData["o_date"]
	sqlord.PatientId = mapData["o_patient_id"]
	if sqlord.PatientId == "" {
		sqlord.PatientId = "0"
	}
	sqlord.FileId = mapData["o_file_id"]
	if sqlord.FileId == "" {
		sqlord.FileId = "0"
	}
	sqlord.FileCode = mapData["o_file_code"]
	sqlord.FileCode2 = mapData["o_file_code2"]
	sqlord.FileDate = mapData["o_file_date"]
	sqlord.RoundNo, err = strconv.Atoi(mapData["o_round_no"])
	if err != nil {
		sqlord.RoundNo = 0
	}
	sqlord.RackNo, err = strconv.Atoi(mapData["o_rack_no"])
	if err != nil {
		sqlord.RackNo = 0
	}
	sqlord.PositionNo, err = strconv.Atoi(mapData["o_position_no"])
	if err != nil {
		sqlord.PositionNo = 0
	}
	sqlord.SampleNo, err = strconv.Atoi(mapData["o_sample_no"])
	if err != nil {
		sqlord.SampleNo = 0
	}
	sqlord.PatientName = mapData["o_patient_name"]

	tmpInt, err := strconv.Atoi(mapData["o_result_type"])

	if err != nil {
		sqlord.ResultType = 0
	} else {
		sqlord.ResultType = SQLOrderResultType(tmpInt)
	}

	sqlord.QCLevelInfo = mapData["o_qc_level_info"]
	sqlord.QCLotInfo = mapData["o_qc_lt_info"]
	sqlord.OrderProgrammedDateTime = mapData["o_programmed_on"]
	sqlord.OrderProgrammedBy = mapData["o_programmed_by"]
	sqlord.ResultReceivedDateTime = mapData["o_result_received_on"]
	sqlord.ResultReceivedBy = mapData["o_result_received_by"]
	sqlord.ResultConfimedDateTime = mapData["o_result_confirmed_on"]
	sqlord.ResultConfimedBy = mapData["o_result_confirmed_by"]

	if testJSON, ok := mapData["o_tests"]; ok {
		tests := []SQLTest{}
		json.Unmarshal([]byte(testJSON), &tests)
		sqlord.Tests = tests
	}

	return nil
}

func (sqlord *SQLOrder) ToMap() (map[string]string, error) {
	tmpMap := make(map[string]string)
	tmpMap["order_id"] = strconv.Itoa(sqlord.Id)
	tmpMap["o_date"] = sqlord.Date
	tmpMap["o_patient_id"] = sqlord.PatientId
	tmpMap["o_file_id"] = sqlord.FileId
	tmpMap["o_file_code"] = sqlord.FileCode
	tmpMap["o_file_code2"] = sqlord.FileCode2
	tmpMap["o_file_date"] = sqlord.FileDate
	tmpMap["o_round_no"] = strconv.Itoa(sqlord.RoundNo)
	tmpMap["o_rack_no"] = strconv.Itoa(sqlord.RackNo)
	tmpMap["o_position_no"] = strconv.Itoa(sqlord.PositionNo)
	tmpMap["o_sample_no"] = strconv.Itoa(sqlord.SampleNo)
	tmpMap["o_patient_name"] = sqlord.PatientName
	tmpMap["o_result_type"] = strconv.Itoa(int(sqlord.ResultType))
	tmpMap["o_qc_level_info"] = sqlord.QCLevelInfo
	tmpMap["o_qc_lt_info"] = sqlord.QCLotInfo
	tmpMap["o_programmed_on"] = sqlord.OrderProgrammedDateTime
	tmpMap["o_programmed_by"] = sqlord.OrderProgrammedBy
	tmpMap["o_result_received_on"] = sqlord.ResultReceivedDateTime
	tmpMap["o_result_received_by"] = sqlord.ResultReceivedBy
	tmpMap["o_result_confirmed_on"] = sqlord.ResultReceivedBy
	tmpMap["o_result_confirmed_by"] = sqlord.ResultConfimedBy

	jsonTxt, err := json.Marshal(sqlord.Tests)
	if err != nil {
		jsonTxt = []byte("[]")
	}
	tmpMap["o_tests"] = string(jsonTxt)

	return tmpMap, nil
}

func GetOrderQueryInterface() []interface{} {
	return []interface{}{ // Standard MySQL columns
		new(int),                //order_id
		new(string),             //o_date
		new(int),                //o_patient_id
		new(int),                //o_file_id
		new(string),             //o_file_code
		new(string),             //o_file_code2
		new(string),             //o_file_date
		new(int),                //o_round_no
		new(int),                //o_rack_no
		new(int),                //o_position_no
		new(int),                //o_sample_no
		new(string),             //o_patient_name
		new(SQLOrderResultType), //o_result_type
		new(string),             //o_qc_level_info
		new(string),             //o_qc_lt_info
		new(string),             //o_programmed_on
		new(string),             //o_programmed_by
		new(string),             //o_result_received_on
		new(string),             //o_result_received_by
		new(string),             //o_result_confirmed_on
		new(string),             //o_result_confirmed_by
		//new(string), //o_tests
	}
}

func GetFilesStats(onDate string) (OrderStats, error) {
	CheckOrdersTable()

	os := OrderStats{}
	tot, err := SQLITEDatabase.ExecTotalQuery("ORDERS", "o_date = ?", onDate)
	if err != nil {
		return os, err
	}
	os.ProgrammedPatients = tot

	tot, err = SQLITEDatabase.ExecExtTotalQuery("select count(*) as tot from TESTS t "+
		"inner join ORDERS o on t.t_order_id = o.order_id where o.o_date = ? "+
		"and (t_curr_result is null or t_curr_result = '') "+
		"and (t_curr_interpretation is null or t_curr_interpretation = '')", onDate)

	if err != nil {
		return os, err
	}
	os.AnalisysInWork = tot

	tot, err = SQLITEDatabase.ExecExtTotalQuery("select count(*) as tot from TESTS t inner join ORDERS o on t.t_order_id = o.order_id where o.o_date = ? and ((t_curr_result is not null and t_curr_result <> '') or (t_curr_interpretation is not null and t_curr_interpretation <> ''))", onDate)
	if err != nil {
		return os, err
	}
	os.FinalizedPatients = tot

	tot, totActive, err := GetKnownTestsNo()
	if err != nil {
		return os, err
	}
	os.WorkingCapabilityTotal = tot
	os.WorkingCapabilityActive = totActive

	return os, nil
}

func GetOrder(orderId int, formatDateToWeb bool) (SQLOrder, error) {
	CheckOrdersTable()
	sqlOrd := SQLOrder{}
	if orderId > 0 {
		dest := GetOrderQueryInterface()
		data, err := SQLITEDatabase.ExecQueryFromTable("ORDERS", fmt.Sprintf("select * from ORDERS where order_id = %d", orderId), dest)
		if err != nil {
			return sqlOrd, err
		}

		if len(data) > 0 {
			err := sqlOrd.ParseFromRawData(data[0], formatDateToWeb)
			if err != nil {
				return SQLOrder{}, err
			}
		}
	}

	return sqlOrd, nil
}

func QueryOrders(dbQuery string, formatDateToWeb bool) ([]SQLOrder, error) {
	CheckOrdersTable()
	dest := GetOrderQueryInterface()

	data, err := SQLITEDatabase.ExecQueryFromTable("ORDERS", dbQuery, dest)
	if err != nil {
		return nil, err
	}

	sqlOrders := []SQLOrder{}
	for i := range data {
		sqlOrder := SQLOrder{}
		err := sqlOrder.ParseFromRawData(data[i], formatDateToWeb)
		if err != nil {
			return []SQLOrder{}, err
		}

		testQuery := SQLTestQuery{OrderId: strconv.Itoa(sqlOrder.Id)}
		sqlOrder.Tests, err = GetTests(testQuery)
		if err != nil {
			return []SQLOrder{}, err
		}

		sqlOrders = append(sqlOrders, sqlOrder)
	}

	return sqlOrders, nil
}
func GetOrders(query SQLOrderQuery, formatDateForWeb bool) ([]SQLOrder, error) {
	CheckOrdersTable()
	dest := GetOrderQueryInterface()

	dbQuery := "select * from ORDERS where (1=1)"
	if query.FromDate != "" {
		dbQuery += " and o_date>='" + query.FromDate + "'"
	}
	if query.ToDate != "" {
		dbQuery += " and o_date<='" + query.ToDate + "'"
	}
	if query.OnDate != "" {
		dbQuery += " and o_date='" + query.OnDate + "'"
	}
	if query.Id != "" {
		dbQuery += " and order_id ='" + query.Id + "'"
	}
	if query.FileId != "" {
		dbQuery += " and o_file_id ='" + query.FileId + "'"
	}
	if query.PatientId != "" {
		dbQuery += " and o_patient_id ='" + query.PatientId + "'"
	}
	if query.RackNo != "" {
		dbQuery += " and o_rack_no ='" + query.RackNo + "'"
	}
	if query.PositionNo != "" {
		dbQuery += " and o_position_no ='" + query.PositionNo + "'"
	}
	if query.Roundno != "" {
		dbQuery += " and o_round_no ='" + query.Roundno + "'"
	}

	data, err := SQLITEDatabase.ExecQueryFromTable("ORDERS", dbQuery, dest)
	if err != nil {
		return nil, err
	}

	sqlOrders := []SQLOrder{}
	for i := range data {
		sqlOrder := SQLOrder{}
		err := sqlOrder.ParseFromRawData(data[i], formatDateForWeb)
		if err != nil {
			return []SQLOrder{}, err
		}

		testQuery := SQLTestQuery{OrderId: strconv.Itoa(sqlOrder.Id)}
		sqlOrder.Tests, err = GetTests(testQuery)
		if err != nil {
			return []SQLOrder{}, err
		}

		sqlOrders = append(sqlOrders, sqlOrder)
	}

	return sqlOrders, nil
}

func SaveOrder(cond SQLOrderQuery, orderData SQLOrder) (bool, int, error) {
	CheckOrdersTable()
	orders, err := GetOrders(cond, false)
	if err != nil {
		return false, 0, err
	}

	var execParams []interface{}
	if len(orders) > 0 {
		//update
		for _, order := range orders {
			if debugSQLWrapperLevel > 10 {
				fmt.Println("Updating oder")
			}
			stmt, err := SQLITEDatabase.DbObj.Prepare("update ORDERS set o_date = ?,o_patient_id=?,o_file_id=?,o_file_code=?,o_file_code2=?,o_file_date=?,o_round_no=?,o_rack_no=?,o_position_no=?,o_sample_no=?,o_patient_name=?,o_result_type=?,o_qc_level_info=?,o_qc_lt_info=?,o_programmed_on=?,o_programmed_by=?,o_result_received_on=?,o_result_received_by=?,o_result_confirmed_on=?,o_result_confirmed_by=?  where order_id = ?")

			if err != nil {
				return false, 0, err
			}
			execParams = []interface{}{orderData.Date, orderData.PatientId, orderData.FileId, orderData.FileCode, orderData.FileCode2, orderData.FileDate, orderData.RoundNo, orderData.RackNo, orderData.PositionNo, orderData.SampleNo, orderData.PatientName, orderData.ResultType, orderData.QCLevelInfo, orderData.QCLotInfo, orderData.OrderProgrammedDateTime, orderData.OrderProgrammedBy, orderData.ResultReceivedDateTime, orderData.ResultReceivedBy, orderData.ResultConfimedDateTime, orderData.ResultConfimedBy, order.Id}

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
			fmt.Println("Inserting order")
		}
		stmt, err := SQLITEDatabase.DbObj.Prepare("insert into ORDERS(o_date,o_patient_id,o_file_id,o_file_code,o_file_code2,o_file_date,o_round_no,o_rack_no,o_position_no,o_sample_no,o_patient_name,o_result_type,o_qc_level_info,o_qc_lt_info,o_programmed_on,o_programmed_by,o_result_received_on,o_result_received_by,o_result_confirmed_on,o_result_confirmed_by) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
		if err != nil {
			return false, 0, err
		}
		execParams = []interface{}{orderData.Date, orderData.PatientId, orderData.FileId, orderData.FileCode, orderData.FileCode2, orderData.FileDate, orderData.RoundNo, orderData.RackNo, orderData.PositionNo, orderData.SampleNo, orderData.PatientName, orderData.ResultType, orderData.QCLevelInfo, orderData.QCLotInfo, orderData.OrderProgrammedDateTime, orderData.OrderProgrammedBy, orderData.ResultReceivedDateTime, orderData.ResultReceivedBy, orderData.ResultConfimedDateTime, orderData.ResultConfimedBy}

		resp, err := stmt.Exec(execParams...)
		if err != nil {
			return false, 0, err
		}
		defer stmt.Close()

		lastInsertId, err := resp.LastInsertId()
		if err != nil {
			return false, 0, err
		}

		//now lets insert the tests
		for _, test := range orderData.Tests {
			test.OrderId = int(lastInsertId)
			_, _, err := SaveTest(SQLTestQuery{OrderId: strconv.Itoa(int(lastInsertId)), Tag: test.Tag}, test)
			if err != nil {
				return false, 0, err
			}
		}
		return true, int(lastInsertId), nil
	}
}

func SaveOrdersBulk(queue *general.ObjectQueue) {
	for queue.Len() > 0 {
		tmpObj, err := queue.Pop()
		if err != nil {
			fmt.Printf("Erroron save result: %v\n", err)
		}
		tmpOrder, ok := tmpObj.(SQLOrder)
		if ok {
			if debugSQLWrapperLevel > 10 {
				fmt.Println("New torder to save %v\n", tmpOrder)
			}
			SaveOrder(SQLOrderQuery{Id: strconv.Itoa(tmpOrder.Id)}, tmpOrder)
		}
	}
}

func SaveOrderFromMap(orderId int, orderData map[string]string) (bool, int, error) {
	ktObj := SQLOrder{}
	ktObj.ParseFromMap(orderData)
	return SaveOrder(SQLOrderQuery{Id: strconv.Itoa(orderId)}, ktObj)
}

func DeleteOrder(orderId int) error {
	CheckOrdersTable()
	if debugSQLWrapperLevel > 10 {
		fmt.Println("Deleting ORDER")
	}

	tsts, err := GetTests(SQLTestQuery{OrderId: strconv.Itoa(orderId)})
	if err != nil {
		fmt.Printf("\n Cannot get tests for order %d \n", orderId)
		return err
	}
	for _, tst := range tsts {
		DeleteTest(tst.Id)
	}

	stmt, err := SQLITEDatabase.DbObj.Prepare("delete from ORDERS where order_id = ?")
	defer stmt.Close()
	if err != nil {
		fmt.Printf("\n Cannot prepare delete from orders %v \n", err)
		return err
	}

	execParams := []interface{}{orderId}

	_, err = stmt.Exec(execParams...)
	if err != nil {
		fmt.Printf("\n Cannot exec delete from orders %v \n", err)
		return err
	}

	return nil
}

func (sqlord *SQLOrder) FormatDatesForWeb() {
	sqlord.Date = general.WebDateFromDB(sqlord.Date)
	sqlord.FileDate = general.WebDateFromDB(sqlord.FileDate)
}
func (sqlord *SQLOrder) FormatDatesForDB() {
	sqlord.Date = general.DBDateFromWeb(sqlord.Date)
	sqlord.FileDate = general.DBDateFromWeb(sqlord.FileDate)
}

func (sqlord *SQLOrder) SaveFromCommToDB() error {
	nowt := time.Now()
	query := SQLOrderQuery{Id: strconv.Itoa(sqlord.Id)}
	_, _, err := SaveOrder(query, *sqlord)
	if err != nil {
		return err
	}

	//now lets insert the tests
	for _, test := range sqlord.Tests {
		if general.LoggedInUser != nil {
			test.ResultReceivedBy = string(general.LoggedInUser.UserId)
		}
		test.ResultReceivedDateTime = nowt.Format("2006-01-02 15:04:05")

		_, _, err := SaveTest(SQLTestQuery{OrderId: strconv.Itoa(sqlord.Id), Code: test.Code}, test)
		if err != nil {
			return err
		}
	}
	return nil
}
