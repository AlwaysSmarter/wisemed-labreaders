package siui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInsuredResponseStatesAndIdentifiers(t *testing.T) {
	for _, state := range []string{"0", "1", "3", "99"} {
		raw := `<?xml version="1.0" encoding="UTF-8"?><insuredResponse xmlns="http://www.cnas.ro/siui/2.0"><insured cid="40172426789613933080" pid="1000000000000" state="` + state + `"/></insuredResponse>`
		result := map[string]any{"values": []string{raw}}
		enrichInsuredResult("getInsured", []string{raw}, result)
		person, ok := result["insured"].(map[string]any)
		if !ok || person["pid"] != "1000000000000" || person["cid"] != "40172426789613933080" || person["state"] != state {
			t.Fatalf("identifiers/state lost: %v", person)
		}
		data, err := json.Marshal(person)
		if err != nil || !strings.Contains(string(data), `"categories":[]`) || !strings.Contains(string(data), `"states":[]`) {
			t.Fatal(string(data), err)
		}
		if result["values"].([]string)[0] != raw {
			t.Fatal("raw response modified")
		}
	}
}
func TestInsuredResponseAllCategoriesAndStates(t *testing.T) {
	raw := `<s:insuredResponse xmlns:s="http://www.cnas.ro/siui/2.0"><s:insured cid="12345678901234567890" pid="1000000000000" state="1" extra="retained"><s:categories><s:category code="3" validFrom="2017-04-04"/><s:category code="9" validFrom="2020-01-01" validTo="2020-12-31"/></s:categories><s:states><s:state code="SAL" validFrom="2017-04-04"/><s:state code="OTHER" validFrom="2021-01-01" validTo="2022-01-01"/></s:states></s:insured></s:insuredResponse>`
	result := map[string]any{}
	enrichInsuredResult("getInsuredByCID", []string{raw}, result)
	person := result["insured"].(map[string]any)
	categories := person["categories"].([]map[string]any)
	states := person["states"].([]map[string]any)
	if len(categories) != 2 || len(states) != 2 || categories[1]["validTo"] != "2020-12-31" || states[0]["code"] != "SAL" || person["extra"] != "retained" {
		t.Fatal(person)
	}
}
func TestInsuredResponseInvalidPreservesRaw(t *testing.T) {
	for _, raw := range []string{"CNAS error", "<insuredResponse>", "<other/>", "<insuredResponse/>", "<insuredResponse><insured/><insured/></insuredResponse>", "<insuredResponse><insured/></insuredResponse><extra/>"} {
		result := map[string]any{"values": []string{raw}}
		enrichInsuredResult("getInsured", []string{raw}, result)
		if result["insured"] != nil || result["parse_error"] == nil || result["values"].([]string)[0] != raw {
			t.Fatal("invalid XML converted to person")
		}
	}
	result := map[string]any{}
	enrichInsuredResult("getRefund", []string{"not XML"}, result)
	if len(result) != 0 {
		t.Fatal("unrelated operation changed")
	}
}
