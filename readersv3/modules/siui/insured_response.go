package siui

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

type insuredEntry struct {
	Attributes []xml.Attr `xml:",any,attr"`
}
type insuredXML struct {
	XMLName xml.Name `xml:"insuredResponse"`
	Insured []struct {
		Attributes []xml.Attr     `xml:",any,attr"`
		Categories []insuredEntry `xml:"categories>category"`
		States     []insuredEntry `xml:"states>state"`
	} `xml:"insured"`
}

func insuredAttributes(attributes []xml.Attr) map[string]any {
	result := map[string]any{}
	for _, attr := range attributes {
		if attr.Name.Local != "xmlns" && attr.Name.Space != "xmlns" {
			result[attr.Name.Local] = attr.Value
		}
	}
	return result
}

// Keep identifiers and CNAS codes as strings (CID exceeds integer precision in JS).
func parseInsuredResponse(value string) (map[string]any, error) {
	var response insuredXML
	decoder := xml.NewDecoder(strings.NewReader(value))
	if err := decoder.Decode(&response); err != nil {
		return nil, errors.New("invalid insuredResponse XML")
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("invalid insuredResponse XML")
		}
		if data, ok := token.(xml.CharData); ok && strings.TrimSpace(string(data)) == "" {
			continue
		}
		if _, ok := token.(xml.Comment); ok {
			continue
		}
		return nil, errors.New("unexpected content after insuredResponse")
	}
	if len(response.Insured) != 1 {
		return nil, errors.New("insuredResponse must contain exactly one insured element")
	}
	person := response.Insured[0]
	result := insuredAttributes(person.Attributes)
	categories := make([]map[string]any, 0, len(person.Categories))
	for _, item := range person.Categories {
		categories = append(categories, insuredAttributes(item.Attributes))
	}
	states := make([]map[string]any, 0, len(person.States))
	for _, item := range person.States {
		states = append(states, insuredAttributes(item.Attributes))
	}
	result["categories"], result["states"] = categories, states
	return result, nil
}

func enrichInsuredResult(operation string, values []string, result map[string]any) {
	if operation != "getInsured" && operation != "getInsuredByCID" {
		return
	}
	if len(values) != 1 {
		result["parse_error"] = "expected one insuredResponse XML value"
		return
	}
	insured, err := parseInsuredResponse(values[0])
	if err != nil {
		// A CNAS business-error string must remain available, not become empty data.
		result["parse_error"] = err.Error()
		return
	}
	result["insured"] = insured
}
