package siui

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

// Check the service in both directions: testing only catalogue entries cannot
// detect a prescription operation accidentally omitted from that catalogue.
func TestCompletePrescriptionWSDLCoverage(t *testing.T) {
	for _, service := range []string{"SiuiWS", "EPrescriptionWS", "SiuiValidateWS"} {
		data, err := assets.ReadFile("assets/wsdl/" + service + ".wsdl")
		if err != nil {
			t.Fatal(err)
		}
		root, err := parseXML(data)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range root.child("portType").Children {
			name := entry.attr("name")
			if service != "EPrescriptionWS" && !strings.Contains(strings.ToLower(name), "prescription") {
				continue
			}
			group := "clinice"
			if service == "SiuiWS" {
				group = "general"
			}
			op, ok := findOperation(group, name)
			if !ok || op.Service != service {
				t.Fatalf("missing %s.%s from %s", group, name, service)
			}
			wantMutation := !strings.HasPrefix(name, "get") && !strings.HasPrefix(name, "download")
			if op.Mutating != wantMutation {
				t.Fatalf("incorrect mutation permissions: %s", name)
			}
		}
	}
}

func TestPrescriptionSeriesBooleanValidation(t *testing.T) {
	op, _ := findOperation("general", "generatePrescriptionSeries")
	args := sampleParameters(op)
	for _, value := range []string{"true", "false", "1", "0"} {
		args["isOnLine"] = value
		if err := validateParameters(op, args); err != nil {
			t.Fatal(value, err)
		}
	}
	for _, value := range []string{"", "TRUE", "yes", "2", " true "} {
		args["isOnLine"] = value
		if validateParameters(op, args) == nil {
			t.Fatalf("invalid XML boolean accepted: %q", value)
		}
	}
}

func TestPrescriptionSeriesPreservesAllReturnValues(t *testing.T) {
	for _, name := range []string{"generatePrescriptionSeries", "getPrescriptionSeriesInfo"} {
		op, _ := findOperation("general", name)
		soap := `<s:Envelope xmlns:s="` + soapNS + `"><s:Body><m:` + name + `Response xmlns:m="` + op.Namespace + `"><` + name + `Return>series-one</` + name + `Return><` + name + `Return>series-two</` + name + `Return><` + name + `Return></` + name + `Return></m:` + name + `Response></s:Body></s:Envelope>`
		values, err := operationValues(op, []byte(soap))
		if err != nil || len(values) != 3 || values[0] != "series-one" || values[1] != "series-two" || values[2] != "" {
			t.Fatal(values, err)
		}
		if !op.Download {
			t.Fatal("series operations must download the CNAS result file")
		}
	}
}

func TestNewPrescriptionRoutesPermissionsAndDeduplication(t *testing.T) {
	b := &operationBackend{t: t}
	_, rt := serviceTestModule(t, b)
	for _, name := range []string{"generatePrescriptionSeries", "getPrescriptionSeriesInfo", "processIssuedPrescription", "getReleasedPrescription", "cancelReleasedPrescription", "updateInvoices"} {
		t.Run(name, func(t *testing.T) {
			group := "clinice"
			if strings.Contains(name, "Series") {
				group = "general"
			}
			op, ok := findOperation(group, name)
			if !ok {
				t.Fatal("missing operation")
			}
			payload, _ := json.Marshal(map[string]any{"operation_id": "test-" + name, "parameters": sampleParameters(op)})
			principal := apibridge.Principal{Subject: "operator", TenantID: "clinic", ConnectionID: "wsm"}
			invoke := func() apibridge.Response {
				result, err := apibridge.Invoke(context.Background(), rt.mux, principal, apibridge.Request{Method: http.MethodPost, Path: "/api/siui/" + group + "/" + name, Body: payload})
				if err != nil {
					t.Fatal(err)
				}
				return result
			}
			before := b.post.Load()
			if op.Mutating {
				if result := invoke(); result.Status != 403 {
					t.Fatal("mutation allowed without admin", result)
				}
				if b.post.Load() != before {
					t.Fatal("unauthorized request contacted CNAS")
				}
			}
			principal.Admin = true
			result := invoke()
			if result.Status != 202 {
				t.Fatal(result)
			}
			raw, _ := json.Marshal(result.Body)
			var reply struct {
				Call serviceCall `json:"call"`
			}
			if err := json.Unmarshal(raw, &reply); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				polled, err := apibridge.Invoke(context.Background(), rt.mux, principal, apibridge.Request{Method: http.MethodGet, Path: "/api/siui/calls/" + reply.Call.ID})
				if err != nil || polled.Status != 200 {
					t.Fatal(polled, err)
				}
				raw, _ = json.Marshal(polled.Body)
				if err = json.Unmarshal(raw, &reply); err != nil {
					t.Fatal(err)
				}
				if reply.Call.Status == "completed" || reply.Call.Status == "failed" || reply.Call.Status == "unknown" {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if reply.Call.Status != "completed" {
				t.Fatalf("call did not complete: %+v", reply.Call)
			}
			if result = invoke(); result.Status != 200 {
				t.Fatal("retry did not reuse call", result)
			}
			if b.post.Load() != before+1 {
				t.Fatal("duplicate SOAP request")
			}
		})
	}
}

func TestNullablePrescriptionParameters(t *testing.T) {
	for _, name := range []string{"getPrescriptionSeriesInfo", "generatePrescriptionSeries"} {
		op, _ := findOperation("general", name)
		args := sampleParameters(op)
		args["subUnitCode"] = ""
		if err := validateParameters(op, args, "subUnitCode"); err != nil {
			t.Fatal(err)
		}
		root, err := parseXML(operationEnvelope(op, args, "subUnitCode"))
		if err != nil {
			t.Fatal(err)
		}
		field := root.child("Body").Children[0].child("subUnitCode")
		if field.attr("nil") != "true" || field.Text != "" {
			t.Fatal("null was not serialized as xsi:nil", field)
		}
		plain, _ := parseXML(operationEnvelope(op, args))
		if plain.child("Body").Children[0].child("subUnitCode").attr("nil") != "" {
			t.Fatal("empty string changed into null")
		}
		if validateParameters(op, args, "uic") == nil {
			t.Fatal("required string accepts null")
		}
	}
}

type prescriptionFileBackend struct{ operationBackend }

func (b *prescriptionFileBackend) Do(ctx context.Context, method, address string, headers http.Header, data []byte) (int, http.Header, []byte, error) {
	status, h, body, err := b.operationBackend.Do(ctx, method, address, headers, data)
	if err != nil || method != "POST" {
		return status, h, body, err
	}
	root, _ := parseXML(data)
	op := root.child("Body").Children[0]
	body = []byte(`<s:Envelope xmlns:s="` + soapNS + `"><s:Body><m:` + op.XMLName.Local + `Response xmlns:m="` + op.XMLName.Space + `"><` + op.XMLName.Local + `Return>/files/series.xml</` + op.XMLName.Local + `Return><` + op.XMLName.Local + `Return>` + strconv.Itoa(len(b.download)) + `</` + op.XMLName.Local + `Return></m:` + op.XMLName.Local + `Response></s:Body></s:Envelope>`)
	return status, h, body, nil
}

func TestPrescriptionNullAndFileDownloadThroughWSM(t *testing.T) {
	for _, name := range []string{"getPrescriptionSeriesInfo", "generatePrescriptionSeries"} {
		t.Run(name, func(t *testing.T) {
			b := &prescriptionFileBackend{operationBackend: operationBackend{t: t, download: []byte(`<series/>`)}}
			b.check = func(_ string, data []byte) {
				root, _ := parseXML(data)
				field := root.child("Body").Children[0].child("subUnitCode")
				if field.attr("nil") != "true" {
					t.Error("SOAP lost null")
				}
			}
			_, rt := serviceTestModule(t, b)
			op, _ := findOperation("general", name)
			parameters := mapStringAny(sampleParameters(op))
			parameters["subUnitCode"] = nil
			principal := apibridge.Principal{Subject: "operator", TenantID: "clinic", ConnectionID: "wsm", Admin: true}
			invoke := func(args map[string]any) apibridge.Response {
				payload, _ := json.Marshal(map[string]any{"operation_id": "nullable-series", "parameters": args})
				res, err := apibridge.Invoke(context.Background(), rt.mux, principal, apibridge.Request{Method: "POST", Path: "/api/siui/general/" + name, Body: payload})
				if err != nil {
					t.Fatal(err)
				}
				return res
			}
			res := invoke(parameters)
			if res.Status != 202 {
				t.Fatal(res)
			}
			var reply struct {
				Call serviceCall `json:"call"`
			}
			raw, _ := json.Marshal(res.Body)
			json.Unmarshal(raw, &reply)
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				res, err := apibridge.Invoke(context.Background(), rt.mux, principal, apibridge.Request{Method: "GET", Path: "/api/siui/calls/" + reply.Call.ID})
				if err != nil || res.Status != 200 {
					t.Fatal(res, err)
				}
				raw, _ = json.Marshal(res.Body)
				json.Unmarshal(raw, &reply)
				if reply.Call.Status == "completed" || reply.Call.Status == "failed" || reply.Call.Status == "unknown" {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if reply.Call.Status != "completed" {
				t.Fatal(reply.Call)
			}
			var result struct {
				Values []string       `json:"values"`
				File   map[string]any `json:"file"`
			}
			json.Unmarshal(reply.Call.Result, &result)
			if len(result.Values) != 2 || result.File["size"] != float64(len(b.download)) {
				t.Fatal(string(reply.Call.Result))
			}
			if res = invoke(parameters); res.Status != 200 || b.post.Load() != 1 {
				t.Fatal("duplicate operation", res)
			}
			parameters["subUnitCode"] = ""
			if res = invoke(parameters); res.Status != 409 {
				t.Fatal("null/empty dedup collision", res)
			}
			parameters["subUnitCode"] = nil
			parameters["uic"] = nil
			if res = invoke(parameters); res.Status != 400 {
				t.Fatal("null uic accepted", res)
			}
		})
	}
}
