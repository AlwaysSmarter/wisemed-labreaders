package wisemed

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	jwt "github.com/dgrijalva/jwt-go"
	"io/ioutil"
	"log"
	"net/http"
	"time"
	"wisemed-labreaders/config"
)

type WMAPIError struct {
	Message      string `json:"message"`
	ErrorCode    string `json:"error_code"`
	ErrorContext string `json:"error_context"`
}
type JWTClaims struct {
	CallerID   string `json:"caller_id"`
	CallerType string `json:"caller_type"`
	jwt.StandardClaims
}

func isWiseMEDAPIConfigOK() (bool, error) {
	if config.ServerConfiguration.WMAPIIP == "" {
		return false, errors.New("WiseMED API missing IP")
	}
	if config.ServerConfiguration.WMAPIPort == "" {
		return false, errors.New("WiseMED API missing port")
	}
	if config.ServerConfiguration.WMAPIPath == "" {
		return false, errors.New("WiseMED API missing path")
	}
	if config.ServerConfiguration.WMAPIProtocol == "" {
		return false, errors.New("WiseMED API missing protocol")
	}
	if config.ServerConfiguration.WMAPIKey == "" {
		return false, errors.New("WiseMED API missing key")
	}
	return true, nil
}

func createJWTToken() (string, error) {
	expirationTime := time.Now().Add(5 * time.Minute)
	claims := &JWTClaims{
		CallerID:   "WM-Lab-Reader",
		CallerType: config.APPAnalyzerType.String(),
		StandardClaims: jwt.StandardClaims{
			// In JWT, the expiry time is expressed as unix milliseconds
			ExpiresAt: expirationTime.Unix(),
		},
	}
	// Declare the token with the algorithm used for signing, and the claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Create the JWT string
	tokenString, err := token.SignedString([]byte(config.ServerConfiguration.WMAPIKey))
	if err != nil {
		return "", err
	}

	fmt.Printf("Created JWT Token : %s\n", tokenString)
	return tokenString, nil
}

func wiseMEDAPIPut(path string, data map[string]string) ([]byte, error) {
	jsonReq, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return wiseMEDAPIPutByteArr(path, jsonReq)
}

func wiseMEDAPIPutByteArr(path string, jsonReq []byte) ([]byte, error) {
	url := fmt.Sprintf("%s://%s:%s%s%s", config.ServerConfiguration.WMAPIProtocol, config.ServerConfiguration.WMAPIIP, config.ServerConfiguration.WMAPIPort, config.ServerConfiguration.WMAPIPath, path)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonReq))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	jwtToken, err := createJWTToken()
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", jwtToken)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	log.Printf("RETURNED BODY:\n %v", bodyBytes)
	if resp.StatusCode != 200 {
		wmErr := WMAPIError{}
		json.Unmarshal(bodyBytes, &wmErr)
		if wmErr.Message != "" {
			return bodyBytes, errors.New(wmErr.Message)
		}
		return bodyBytes, errors.New(fmt.Sprintf("%s %s", resp.StatusCode, resp.Status))
	}

	return bodyBytes, nil
}

func wiseMEDAPIGet(path string, data map[string]string) ([]byte, error) {
	jsonReq, err := json.Marshal(data)
	url := fmt.Sprintf("%s://%s:%s%s%s", config.ServerConfiguration.WMAPIProtocol, config.ServerConfiguration.WMAPIIP, config.ServerConfiguration.WMAPIPort, config.ServerConfiguration.WMAPIPath, path)
	req, err := http.NewRequest(http.MethodGet, url, bytes.NewBuffer(jsonReq))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	jwtToken, err := createJWTToken()
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", jwtToken)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		wmErr := WMAPIError{}
		json.Unmarshal(bodyBytes, &wmErr)
		if wmErr.Message != "" {
			return bodyBytes, errors.New(wmErr.Message)
		}
		return nil, errors.New(fmt.Sprintf("%s %s", resp.StatusCode, resp.Status))
	}

	return bodyBytes, nil
}
