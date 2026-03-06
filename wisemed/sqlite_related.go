package wisemed

import (
	"strconv"
	"time"
	"wisemed-labreaders/config"
	"wisemed-labreaders/general"
	"wisemed-labreaders/sqlitewrapper"
)

func LoadFileFromWMAsObj(workDate string, round int, rack int, pos int, fileId int) (*sqlitewrapper.SQLOrder, int, error) {
	query := sqlitewrapper.SQLOrderQuery{FileId: strconv.Itoa(fileId), OnDate: workDate}
	transformDateToWeb := false
	alreadyLoadedOrder, err := sqlitewrapper.GetOrders(query, transformDateToWeb)
	if err != nil {
		return nil, 301, err
	}

	if len(alreadyLoadedOrder) <= 0 {
		//load file Data from API
		fileData, err := WMAPIAnalyzerFileData(fileId)
		if err != nil {
			return nil, 300, err
		}
		fileData.FileDate = general.DBDateFromWeb(fileData.FileDate)

		if (round <= 0) || (rack <= 0) || (pos <= 0) {
			//calculate the next available po/rack/round
			round = 1
			rack = 1
			pos = 1
			//get max used round for today
			tmpOrders, err := sqlitewrapper.QueryOrders("select * from ORDERS where o_date = '"+workDate+"' and o_file_id > 0 order by o_round_no desc limit 1", transformDateToWeb)
			if err != nil {
				return nil, 301, err
			}
			if len(tmpOrders) > 0 {
				tmpOrder := tmpOrders[0]
				if tmpOrder.RoundNo > round {
					round = tmpOrder.RoundNo
				}
			}

			//get max used rack for today for the last round
			tmpOrders, err = sqlitewrapper.QueryOrders("select * from ORDERS where o_date = '"+workDate+"' and o_file_id > 0 and o_round_no = "+strconv.Itoa(round)+" order by o_rack_no desc limit 1", transformDateToWeb)
			if err != nil {
				return nil, 301, err
			}
			if len(tmpOrders) > 0 {
				tmpOrder := tmpOrders[0]
				if tmpOrder.RackNo > rack {
					rack = tmpOrder.RackNo
				}
				tmpRacksNo, err := strconv.Atoi(config.ServerConfiguration.WMLRRacksNo)
				if err != nil {
					tmpRacksNo = rack
				}
				if rack > tmpRacksNo {
					round++
					rack = 1
					pos = 1
				}
			}

			//get max used pos no for today for the last round and last rack used
			tmpOrders, err = sqlitewrapper.QueryOrders("select * from ORDERS where o_date = '"+workDate+"' and o_file_id > 0 and o_round_no = "+strconv.Itoa(round)+" and o_rack_no = "+strconv.Itoa(rack)+" order by o_position_no desc limit 1", transformDateToWeb)
			if err != nil {
				return nil, 301, err
			}
			if len(tmpOrders) > 0 {
				tmpOrder := tmpOrders[0]
				if tmpOrder.PositionNo > pos {
					pos = tmpOrder.PositionNo
				}
				pos++
				tmpPosNo, err := strconv.Atoi(config.ServerConfiguration.WMLRPositionsPerRacks)
				if err != nil {
					tmpPosNo = pos
				}
				if pos > tmpPosNo {
					rack++
					pos = 1
				}
				tmpRacksNo, err := strconv.Atoi(config.ServerConfiguration.WMLRRacksNo)
				if err != nil {
					tmpRacksNo = rack
				}
				if rack > tmpRacksNo {
					round++
					rack = 1
					pos = 1
				}
			}
		}

		fileData.Date = workDate
		fileData.RoundNo = round
		fileData.RackNo = rack
		fileData.PositionNo = pos
		_, _, err = sqlitewrapper.SaveOrder(query, *fileData)
		if err != nil {
			return nil, 302, err
		}
	}

	//now reload formatted
	query = sqlitewrapper.SQLOrderQuery{FileId: strconv.Itoa(fileId), OnDate: workDate}
	alreadyLoadedOrder, err = sqlitewrapper.GetOrders(query, true)
	if err != nil {
		return nil, 301, err
	}

	return &alreadyLoadedOrder[0], 0, nil

}

func LoadFileFromWM(workDate string, round int, rack int, pos int, fileId int) (map[string]string, int, error) {
	order, errCode, err := LoadFileFromWMAsObj(workDate, round, rack, pos, fileId)
	if err != nil {
		return nil, errCode, err
	}
	orderMap, err := order.ToMap()
	if err != nil {
		return nil, 304, err
	}
	return orderMap, 0, nil
}

func ConfirmFilesServicesResults(workDate string, fileId int, userId int) ([]byte, error) {
	query := sqlitewrapper.SQLOrderQuery{OnDate: workDate}
	if fileId > 0 {
		query.FileId = strconv.Itoa(fileId)
	}

	alreadyLoadedOrder, err := sqlitewrapper.GetOrders(query, true)
	if err != nil {
		return nil, err
	}

	fileServResults := []config.WMLRFileServiceResult{}
	for _, order := range alreadyLoadedOrder {
		for _, test := range order.Tests {
			if test.Result == "" && test.Raw != "" {
				test.Result = test.Raw
			}
			//skip tests that do not have a result yet
			if test.Result == "" && test.Interpretation == "" {
				continue
			}

			fsr := config.WMLRFileServiceResult{}
			fsr.FileId = order.FileId
			fsr.ServiceTag = test.Tag
			fsr.AnalyzerNameOnReport = config.ServerConfiguration.WMLRNameOnReport
			fsr.ReagentsSet = test.ResultReagentsSet
			fsr.MeasuringUnit = test.ResultMeasureUnit
			fsr.Result = test.Result
			fsr.Interpretation = test.Interpretation
			fsr.WorkingDatetime = test.ResultReceivedDateTime
			fsr.WorkedBy = test.ResultReceivedBy
			nowt := time.Now()
			fsr.ConfirmationDatetime = nowt.Format("2006-01-02 15:04:05")
			fsr.ConfirmedBy = strconv.Itoa(userId)
			if fsr.WorkedBy == "0" || fsr.WorkedBy == "" {
				fsr.WorkedBy = fsr.ConfirmedBy
			}
			fileServResults = append(fileServResults, fsr)
		}

	}

	updatedData, err := WMAPIConfirmFileServiceResults(fileServResults)
	if err != nil {
		return nil, err
	}

	return updatedData, nil

}
