package siui

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const insuredNS = "http://webservices.utils.svapnt.siveco.ro"
const localInsuredXML = `<insuredResponse xmlns="http://www.cnas.ro/siui/2.0"><insured pid="0000000000000" state="1"/></insuredResponse>`

type InsuredResult struct {
	State   int    `json:"state"`
	Label   string `json:"label"`
	Insured *bool  `json:"insured"`
	XML     string `json:"xml"`
}

func insuredEnvelope(cnp, date string) []byte {
	var b bytes.Buffer
	b.WriteString(`<soap:Envelope xmlns:soap="` + soapNS + `" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema"><soap:Body><m:getInsured xmlns:m="` + insuredNS + `" soap:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><pid xsi:type="xsd:string">`)
	xml.EscapeText(&b, []byte(cnp))
	b.WriteString(`</pid><requestDate xsi:type="xsd:dateTime">`)
	xml.EscapeText(&b, []byte(date))
	b.WriteString(`</requestDate></m:getInsured></soap:Body></soap:Envelope>`)
	return b.Bytes()
}
func checkInsuredInput(cnp, date string) error {
	if len(cnp) != 13 || strings.IndexFunc(cnp, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return errors.New("CNP must contain exactly 13 digits")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return errors.New("date must be YYYY-MM-DD")
	}
	return nil
}
func parseInsured(raw, cnp string) (InsuredResult, error) {
	n, err := parseXML([]byte(raw))
	if err != nil {
		return InsuredResult{}, err
	}
	if n.XMLName.Local != "insuredResponse" || n.XMLName.Space != piasNS {
		return InsuredResult{}, errors.New("invalid insured response")
	}
	person := n.child("insured")
	if pid := person.attr("pid"); pid != "" && pid != cnp {
		return InsuredResult{}, errors.New("CNAS returned a different patient")
	}
	state, err := strconv.Atoi(person.attr("state"))
	if err != nil {
		return InsuredResult{}, errors.New("missing insured state")
	}
	labels := map[int]string{-1: "Eroare CNAS", 0: "Inexistent", 1: "Asigurat", 2: "Neasigurat", 3: "Decedat"}
	label, ok := labels[state]
	if !ok {
		return InsuredResult{}, errors.New("unknown insured state")
	}
	result := InsuredResult{State: state, Label: label, XML: raw}
	if state == 1 || state == 2 {
		insured := state == 1
		result.Insured = &insured
	}
	return result, nil
}
func (c *client) getInsured(ctx context.Context, cnp, date string) (InsuredResult, error) {
	if err := checkInsuredInput(cnp, date); err != nil {
		return InsuredResult{}, err
	}
	h, err := c.sessionHeaders(ctx)
	if err != nil {
		return InsuredResult{}, err
	}
	status, _, body, err := c.backend.Do(ctx, "POST", strings.TrimRight(c.cfg.BaseURL, "/")+"/svapntws/services/SiuiInsuredWS", h, insuredEnvelope(cnp, date+"T00:00:00"))
	if err != nil {
		return InsuredResult{}, err
	}
	if status != 200 && status != 500 {
		return InsuredResult{}, fmt.Errorf("CNAS insured lookup HTTP %d", status)
	}
	raw, err := decodeOperationSOAP(body, "getInsured", insuredNS)
	if err != nil {
		return InsuredResult{}, err
	}
	if status != 200 {
		return InsuredResult{}, fmt.Errorf("CNAS insured lookup HTTP %d", status)
	}
	if err = c.backend.Validate(ctx, "GetInsuredResponse.xsd", []byte(raw)); err != nil {
		return InsuredResult{}, err
	}
	return parseInsured(raw, cnp)
}
func (m *Module) insuredHTTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode string `json:"mode"`
		CNP  string `json:"cnp"`
		Date string `json:"date"`
	}
	if err := decode(w, r, &in); err != nil {
		fail(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	if in.Mode == "local" {
		// Fixed synthetic fixture only; never construct a native transport or use
		// caller-supplied identifiers. This test cannot submit anything to CNAS.
		if err := validateSchema(ctx, "GetInsuredResponse.xsd", []byte(localInsuredXML)); err != nil {
			fail(w, 503, err.Error())
			return
		}
		result, err := parseInsured(localInsuredXML, "0000000000000")
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		respond(w, 200, map[string]any{"ok": true, "mode": "local", "simulated": true, "cnas_contacted": false, "message": "TEST LOCAL — răspuns fictiv, fără conexiune CNAS. Nu indică starea reală a unei persoane.", "sample_result": result})
		return
	}
	if in.Mode != "cnas" {
		fail(w, 400, "select mode local or cnas explicitly")
		return
	}
	if err := checkInsuredInput(in.CNP, in.Date); err != nil {
		fail(w, 400, err.Error())
		return
	}
	c, release, ok := m.passiveClient(w)
	if !ok {
		return
	}
	defer release()
	result, err := c.getInsured(ctx, in.CNP, in.Date)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	respond(w, 200, map[string]any{"ok": true, "mode": "cnas", "simulated": false, "cnas_contacted": true, "date": in.Date, "result": result})
}
