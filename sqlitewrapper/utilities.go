package sqlitewrapper

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	//"fmt"
	_ "github.com/mattn/go-sqlite3"
)

type SqliteAppParams struct {
	DBSufixPath string
	SqlitePath  string
	ResetCerts  string
}

type SQLColumnDef struct {
	Name string
	Type string
}

var tableFields map[string][]SQLColumnDef
var SQLITEDatabase *SQLiteDatabase = &SQLiteDatabase{}
var SQLITEAPPParams = SqliteAppParams{}
var debugSQLWrapperLevel = 0

func init() {
	tableFields = make(map[string][]SQLColumnDef)

	if SQLITEAPPParams.SqlitePath == "" {
		var err error
		SQLITEAPPParams.SqlitePath, err = filepath.Abs(filepath.Dir(os.Args[0])) //get the current working directory
		if err != nil {
			SQLITEAPPParams.SqlitePath = ""
		}
		SQLITEAPPParams.SqlitePath = fmt.Sprintf("%s%s", SQLITEAPPParams.SqlitePath, string(os.PathSeparator))
	}
}
func OpenSQLLiteDatabase() error {
	dbPath := fmt.Sprintf("%s%sconfig_%s.db", SQLITEAPPParams.SqlitePath, string(os.PathSeparator), SQLITEAPPParams.DBSufixPath)
	if debugSQLWrapperLevel > 0 {
		fmt.Printf("\nUsing database: %s", dbPath)
	}
	err := SQLITEDatabase.OpenDatabase(dbPath)
	if err != nil {
		return err
	}
	return nil
}

type SQLiteDatabase struct {
	DbName string
	DbObj  *sql.DB
}

func (sqldb *SQLiteDatabase) OpenDatabase(dbName string) error {
	sqldb.DbName = dbName
	db, err := sql.Open("sqlite3", sqldb.DbName)
	if err != nil {
		return err
	}

	sqldb.DbObj = db
	return nil
}

func (sqldb *SQLiteDatabase) CloseDatabase() {
	if sqldb.DbObj != nil {
		sqldb.DbObj.Close()
	}
}

func (sqldb *SQLiteDatabase) CheckTableObj(tableName string, tableDef string) error {
	stmt, err := sqldb.DbObj.Prepare("SELECT count(*) as tot FROM sqlite_master WHERE type='table' AND name=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	rows, err := stmt.Query(tableName)
	if err != nil {
		return err
	}

	rows.Next()
	var tot int
	rows.Scan(&tot)
	rows.Close()

	if tot <= 0 {
		//I have to create the table
		if debugSQLWrapperLevel > 0 {
			fmt.Printf("Creating table... %s\n", tableDef)
		}
		_, err := sqldb.DbObj.Exec(tableDef)
		if err != nil {
			fmt.Printf("Creating table error %s\n", err)
			return err
		}
	}

	return nil
}

func (sqldb *SQLiteDatabase) ExecTotalQuery(tableName string, conditionStr string, conditionParams ...interface{}) (int, error) {
	var rows *sql.Rows
	var err error

	if conditionStr != "" {
		stmt, err := SQLITEDatabase.DbObj.Prepare(fmt.Sprintf("select count(*) as tot from %s where %s", tableName, conditionStr))
		defer stmt.Close()

		if err != nil {
			return -1, err
		}
		rows, err = stmt.Query(conditionParams...)
	} else {
		rows, err = SQLITEDatabase.DbObj.Query(fmt.Sprintf("select count(*) as tot from %s", tableName))
	}

	if err != nil {
		return -1, err
	}

	var tot int = 0
	if rows.Next() {
		rows.Scan(&tot)
	}
	rows.Close()

	return tot, nil
}

func (sqldb *SQLiteDatabase) ExecExtTotalQuery(query string, conditionParams ...interface{}) (int, error) {
	stmt, err := SQLITEDatabase.DbObj.Prepare(query)
	if err != nil {
		return -1, err
	}
	rows, err := stmt.Query(conditionParams...)
	if err != nil {
		return -1, err
	}
	defer stmt.Close()
	defer rows.Close()

	var tot int = 0
	if rows.Next() {
		rows.Scan(&tot)
	}

	return tot, nil
}

func (sqldb *SQLiteDatabase) ReturnTableFields(tableName string) ([]SQLColumnDef, error) {
	if tableFields[tableName] == nil {
		rows, err := SQLITEDatabase.DbObj.Query("select name, type FROM PRAGMA_TABLE_INFO('" + tableName + "')")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		sqlCols := []SQLColumnDef{}
		colName := ""
		colType := ""
		for rows.Next() {
			rows.Scan(&colName, &colType)
			tmpCol := SQLColumnDef{Name: colName, Type: colType}
			sqlCols = append(sqlCols, tmpCol)
		}
		tableFields[tableName] = sqlCols
	}

	return tableFields[tableName], nil
}
func (sqldb *SQLiteDatabase) ExecQueryFromTable(tableName string, query string, dest []interface{}, conditionParams ...string) ([]map[string]interface{}, error) {
	var rows *sql.Rows
	var err error
	if len(conditionParams) > 0 {
		stmt, err := SQLITEDatabase.DbObj.Prepare(query)
		defer stmt.Close()
		if err != nil {
			return nil, err
		}
		rows, err = stmt.Query(conditionParams)
		if err != nil {
			return nil, err
		}
	} else {
		rows, err = SQLITEDatabase.DbObj.Query(query)
		if err != nil {
			return nil, err
		}
	}

	tableFields, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	returnedRows := []map[string]interface{}{}
	for rows.Next() {
		retunedRow := make(map[string]interface{})
		rows.Scan(dest...)
		for i := range tableFields {
			retunedRow[tableFields[i]] = reflect.Indirect(reflect.ValueOf(dest[i])).Interface() //convert to interface before sending back
		}
		returnedRows = append(returnedRows, retunedRow)
	}

	return returnedRows, nil
}
func (sqldb *SQLiteDatabase) ExecQueryFromTableNoReturn(query string, conditionParams ...string) error {
	var err error
	if len(conditionParams) > 0 {
		stmt, err := SQLITEDatabase.DbObj.Prepare(query)
		if err != nil {
			return err
		}
		defer stmt.Close()

		_, err = stmt.Query(conditionParams)
		if err != nil {
			return err
		}
	} else {
		_, err = SQLITEDatabase.DbObj.Query(query)
		if err != nil {
			return err
		}
	}

	return nil
}

func DerefString(s *string) string {
	if s != nil {
		return *s
	}

	return ""
}
