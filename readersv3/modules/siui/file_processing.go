package siui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"strings"
)

const processedFileLimit = 64 * 1024 * 1024

// Content preserves mixed text/element order; identifiers and attribute values stay strings.
type fileXMLNode struct {
	Name       string             `json:"name"`
	Namespace  string             `json:"namespace,omitempty"`
	Attributes []fileXMLAttribute `json:"attributes,omitempty"`
	Content    []any              `json:"content,omitempty"`
}
type fileXMLAttribute struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Value     string `json:"value"`
}

func fileXMLJSON(ctx context.Context, data []byte) ([]byte, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var root *fileXMLNode
	var stack []*fileXMLNode
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("invalid XML")
		}
		switch t := token.(type) {
		case xml.Directive:
			return nil, errors.New("XML directives are forbidden")
		case xml.StartElement:
			if len(stack) >= 40 {
				return nil, errors.New("XML nesting too deep")
			}
			n := &fileXMLNode{Name: t.Name.Local, Namespace: t.Name.Space}
			for _, a := range t.Attr {
				n.Attributes = append(n.Attributes, fileXMLAttribute{a.Name.Local, a.Name.Space, a.Value})
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("one XML root required")
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				p.Content = append(p.Content, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if strings.TrimSpace(string(t)) != "" {
					return nil, errors.New("unexpected XML text")
				}
			} else {
				p := stack[len(stack)-1]
				p.Content = append(p.Content, string(t))
			}
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, errors.New("one XML root required")
	}
	return json.Marshal(root)
}
func processCallFile(ctx context.Context, data []byte, q url.Values, serverSchema ...string) ([]byte, error) {
	if len(data) > processedFileLimit {
		return nil, errors.New("file too large")
	}
	unzip := q.Get("unzip")
	if unzip != "" && unzip != "true" && unzip != "false" {
		return nil, errors.New("invalid unzip option")
	}
	if q.Get("entry") != "" && unzip != "true" {
		return nil, errors.New("entry requires unzip=true")
	}
	if unzip == "true" {
		archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, errors.New("invalid ZIP archive")
		}
		var selected *zip.File
		for _, f := range archive.File {
			if f.FileInfo().IsDir() {
				continue
			}
			if q.Get("entry") != "" && q.Get("entry") != f.Name {
				continue
			}
			if selected != nil {
				return nil, errors.New("ambiguous ZIP: specify entry")
			}
			selected = f
		}
		if selected == nil {
			return nil, errors.New("ZIP entry not found")
		}
		if selected.UncompressedSize64 > processedFileLimit {
			return nil, errors.New("ZIP entry too large")
		}
		reader, err := selected.Open()
		if err != nil {
			return nil, errors.New("cannot read ZIP entry")
		}
		data, err = io.ReadAll(io.LimitReader(reader, processedFileLimit+1))
		reader.Close()
		if err != nil {
			return nil, errors.New("invalid ZIP entry")
		}
		if len(data) > processedFileLimit {
			return nil, errors.New("ZIP entry too large")
		}
	}
	if q.Has("schema") {
		return nil, errors.New("XSD schema is selected by the server")
	}
	validation := q.Get("validateXSD")
	if validation != "" && validation != "true" && validation != "false" {
		return nil, errors.New("invalid validateXSD option")
	}
	if validation != "false" {
		schema := ""
		if len(serverSchema) > 0 {
			schema = serverSchema[0]
		}
		if schema == "" {
			return nil, errors.New("XSD validation unavailable for this operation/document; use validateXSD=false to explicitly skip")
		}
		if err := validateSchema(ctx, schema, data); err != nil {
			return nil, err
		}
	}
	switch q.Get("format") {
	case "", "bytes":
	case "xml":
		if _, err := fileXMLJSON(ctx, data); err != nil {
			return nil, err
		}
	case "json":
		var err error
		data, err = fileXMLJSON(ctx, data)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported file format")
	}
	if len(data) > processedFileLimit {
		return nil, errors.New("processed file too large")
	}
	return data, nil
}

// Only server-owned operation context selects a schema; never infer from a client filename.
func resultFileSchema(operation string, args map[string]string) string {
	switch operation {
	case "getProviderInfo", "getProviderInfoForPhysician":
		suffix := map[string]string{"MF": "MF", "CLIN": "Clinic", "PARA": "Para", "STOM": "Stoma", "AMB": "Ambulance", "MD": "Dispozitive", "HC": "Ingrijiri", "SPT": "Hospital", "NHP": "NHP", "DIA": "Dialysis", "SICK": "CM", "CBRET": "CBRET"}[args["partnerCategory"]]
		if suffix != "" {
			return "PersonalizedFile" + suffix + ".xsd"
		}
	case "getInsured", "getInsuredByCID":
		return "GetInsuredResponse.xsd"
	case "getPrescription":
		return "GetPrescriptionResponse.xsd"
	case "getStatusForPrescriptions":
		return "getStatusForPrescriptionsResponse.xsd"
	case "validateReport":
		switch args["reportType"] {
		case "CLIN":
			return "ClinicSpecialityServicesValidateResponse.xsd"
		case "PARA":
			return "ParaclinicServicesValidateResponse.xsd"
		}
	case "validatePrescription":
		return "PhysicianPrescriptionsValidateResponse.xsd"
	case "validateClinicReferral", "validateLabReferral":
		return "PhysicianReferralsValidateResponse.xsd"
	case "validateSickLeave":
		return "PhysicianCertificatesValidateResponse.xsd"
	case "processPrescribedPrescription":
		return "PhysicianDrugPEResponse.xsd"
	case "processHospitalPrescription":
		return "HospitalRegisterPEResponse.xsd"
	case "getPrescribedPrescriptionsForCid":
		return "getPrescribedPrescriptionsForCidResponse.xsd"
	}
	return ""
}
