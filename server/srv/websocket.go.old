package srv

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"strconv"
	"wisemed-labreaders/general"
	"wisemed-labreaders/sqlitewrapper"
	"wisemed-labreaders/wisemed"
)

// define a reader which will listen for
// new messages being sent to our WebSocket
// endpoint
func reader(conn *websocket.Conn) {
	for {
		// read in a message
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		}
		// print out that message for clarity
		action := make(map[string]interface{})
		json.Unmarshal(p, &action)

		actionResp := parseActionMessage(action)
		resp, _ := json.Marshal(actionResp)
		if err := conn.WriteMessage(messageType, resp); err != nil {
			log.Println(err)
			return
		}

	}
}
func handleDelFile(action map[string]interface{}) map[string]interface{} {
	workDate, err := returnStringFromParam("work_date", action)
	if err != nil {
		return wsRespondWithError(201, fmt.Sprintf("%s - %s", "Work date", err.Error()))
	}
	workDate = general.DBDateFromWeb(workDate)
	fileId, err := returnIntegerFromParam("file_id", action)
	if err != nil {
		return wsRespondWithError(205, fmt.Sprintf("%s - %s", "File ID", err.Error()))
	}
	if fileId <= 0 {
		return wsRespondWithError(205, fmt.Sprintf("You have to provide a file ID to delete, not %d", fileId))
	}

	query := sqlitewrapper.SQLOrderQuery{FileId: strconv.Itoa(fileId), OnDate: workDate}
	delSQLOrder, err := sqlitewrapper.GetOrders(query, false)
	if err != nil {
		return wsRespondWithError(205, fmt.Sprintf("Cannot get the order for file ID %d.\nError:", fileId, err.Error()))
	}
	if len(delSQLOrder) <= 0 {
		return wsRespondWithError(205, fmt.Sprintf("Given file ID %d is not progremmed on the analyzer for %s", fileId, workDate))
	}

	err = sqlitewrapper.DeleteOrder(delSQLOrder[0].Id)
	if err != nil {
		return wsRespondWithError(205, fmt.Sprintf("Cannot delete specified order ID %d for file ID %d on %s.\nError: %s", delSQLOrder[0].Id, fileId, workDate, err.Error()))
	}

	orderMap, err := delSQLOrder[0].ToMap()
	if err != nil {
		return wsRespondWithError(205, err.Error())
	}

	action["fileData"] = orderMap
	return action
}

func parseActionMessage(action map[string]interface{}) map[string]interface{} {
	action["success"] = true

	switch action["action"].(string) {
	case "ping":
		action["action"] = "pong"
		return action
	case "getorders":
		workDate, err := returnStringFromParam("work_date", action)
		if err != nil {
			return wsRespondWithError(201, fmt.Sprintf("%s - %s", "Work date", err.Error()))
		}
		workDate = general.DBDateFromWeb(workDate)
		round, err := returnIntegerFromParam("round", action)
		if err != nil {
			return wsRespondWithError(202, fmt.Sprintf("%s - %s", "Round", err.Error()))
		}
		rack, err := returnIntegerFromParam("round", action)
		if err != nil {
			return wsRespondWithError(203, fmt.Sprintf("%s - %s", "Rack", err.Error()))
		}
		orderQuery := sqlitewrapper.SQLOrderQuery{OnDate: workDate, Roundno: strconv.Itoa(round), RackNo: strconv.Itoa(rack)}

		orders, err := sqlitewrapper.GetOrders(orderQuery, true)
		if err != nil {
			return wsRespondWithError(101, fmt.Sprintf("%s - %s", "Cannot get orders", err.Error()))
		}

		ordersResult := make(map[int]sqlitewrapper.SQLOrder)
		for _, order := range orders {
			ordersResult[order.PositionNo] = order
		}
		action["orders"] = ordersResult
		return action
	case "loadfile":
		workDate, err := returnStringFromParam("work_date", action)
		if err != nil {
			return wsRespondWithError(201, fmt.Sprintf("%s - %s", "Work date", err.Error()))
		}
		workDate = general.DBDateFromWeb(workDate)
		round, err := returnIntegerFromParam("round", action)
		if err != nil {
			return wsRespondWithError(202, fmt.Sprintf("%s - %s", "Round", err.Error()))
		}
		rack, err := returnIntegerFromParam("round", action)
		if err != nil {
			return wsRespondWithError(203, fmt.Sprintf("%s - %s", "Rack", err.Error()))
		}
		position, err := returnIntegerFromParam("position", action)
		if err != nil {
			return wsRespondWithError(204, fmt.Sprintf("%s - %s", "Position", err.Error()))
		}
		fileId, err := returnIntegerFromParam("file_id", action)
		if err != nil {
			return wsRespondWithError(205, fmt.Sprintf("%s - %s", "File ID", err.Error()))
		}

		fileData, errCode, err := wisemed.LoadFileFromWM(workDate, round, rack, position, fileId)
		if err != nil {
			return wsRespondWithError(errCode, err.Error())
		}
		action["fileData"] = fileData
		return action
	case "delfile":
		return handleDelFile(action)
	case "confirmone":
		workDate, err := returnStringFromParam("work_date", action)
		if err != nil {
			return wsRespondWithError(201, fmt.Sprintf("%s - %s", "Work date", err.Error()))
		}
		workDate = general.DBDateFromWeb(workDate)
		fileId, err := returnIntegerFromParam("file_id", action)
		if err != nil {
			fileId = 0
		}
		userId, err := returnIntegerFromParam("loggedin_user_id", action)
		if err != nil {
			return wsRespondWithError(201, fmt.Sprintf("%s - %s", "Confirmation user id unknown", err.Error()))
		}

		udatedData, err := wisemed.ConfirmFilesServicesResults(workDate, fileId, userId)
		if err != nil {
			action["error"] = err.Error()
			action["success"] = false
		} else {
			udatedDataArr := map[string]interface{}{}
			err = json.Unmarshal(udatedData, &udatedDataArr)
			if err != nil {
				action["error"] = err.Error()
				action["success"] = false
			} else {
				for k, v := range udatedDataArr {
					action[k] = v
				}
				action["success"] = true
			}
		}
		return action
	case "confirmall":
		workDate, err := returnStringFromParam("work_date", action)
		if err != nil {
			return wsRespondWithError(201, fmt.Sprintf("%s - %s", "Work date", err.Error()))
		}
		workDate = general.DBDateFromWeb(workDate)
		userId, err := returnIntegerFromParam("loggedin_user_id", action)
		if err != nil {
			return wsRespondWithError(201, fmt.Sprintf("%s - %s", "Confirmation user id unknown", err.Error()))
		}

		udatedData, err := wisemed.ConfirmFilesServicesResults(workDate, 0, userId)
		if err != nil {
			action["error"] = err.Error()
			action["success"] = false
		} else {
			udatedDataArr := map[string]interface{}{}
			err = json.Unmarshal(udatedData, &udatedDataArr)
			if err != nil {
				action["error"] = err.Error()
				action["success"] = false
			} else {
				for k, v := range udatedDataArr {
					action[k] = v
				}
				action["success"] = true
			}
		}
		return action
		break
	default:
		return wsRespondWithError(100, "Unknown action")
		return action
	}

	return action
}

func wsRespondWithError(errCode int, errTxt string) map[string]interface{} {
	return map[string]interface{}{
		"success":    false,
		"error_code": errCode,
		"error":      errTxt,
	}
}
func returnIntegerFromParam(key string, data map[string]interface{}) (int, error) {
	var result int
	var err error
	if val, ok := data[key]; ok {
		result, err = strconv.Atoi(fmt.Sprintf("%s", val))
		if err != nil {
			return 0, errors.New("Number expected")
		}
	} else {
		return 0, errors.New("Value should not be void")
	}

	return result, nil
}

func returnStringFromParam(key string, data map[string]interface{}) (string, error) {
	var result string
	if val, ok := data[key]; ok {
		result = fmt.Sprintf("%s", val)
		if result == "" {
			return "", errors.New("Value should not be empty")
		}
	} else {
		return "", errors.New("Value should not be void")
	}

	return result, nil
}
