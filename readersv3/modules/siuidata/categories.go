package siuidata

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type partnerCategory struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Group     string `json:"group"`
	Supported bool   `json:"supported"`
}

var partnerCategories = []partnerCategory{
	{"PHYS", "Medici CNAS și OPSNAJ", "Medici", true},
	{"MF", "Medicină de familie", "Asistență medicală", false}, {"PHM", "Farmacii", "Medicamente", true},
	{"CLIN", "Ambulatoriu clinic", "Asistență medicală", true}, {"PARA", "Paraclinic", "Investigații", true},
	{"STOM", "Stomatologie", "Asistență medicală", false}, {"AMB", "Ambulanță", "Servicii și îngrijiri", false},
	{"MD", "Dispozitive medicale", "Servicii și îngrijiri", false}, {"HC", "Îngrijiri la domiciliu", "Servicii și îngrijiri", false},
	{"REC", "Recuperare", "Servicii și îngrijiri", false}, {"SPT", "Spitale", "Asistență medicală", false},
	{"NHP", "Programe naționale de sănătate", "Programe și raportare", false}, {"DIA", "Dializă", "Programe și raportare", false},
	{"SICK", "Concedii medicale", "Programe și raportare", false}, {"CBRET", "Recuperare și reabilitare", "Programe și raportare", false},
}

func knownPartnerCategory(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	for _, category := range partnerCategories {
		if category.Code == value {
			return value, nil
		}
	}
	return "", errors.New("unknown partnerCategory")
}

func (m *Module) categoriesHTTP(w http.ResponseWriter, r *http.Request) {
	f, e := parseFilters(r.URL.Query())
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	limit, start, e := pagination(f)
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	selected := []partnerCategory{}
	for _, c := range partnerCategories {
		match := true
		for _, field := range []struct{ name, value string }{{"code", c.Code}, {"partnerCategory", c.Code}, {"group", c.Group}, {"name", c.Name}, {"supported", strconv.FormatBool(c.Supported)}} {
			values := f[field.name]
			if len(values) == 0 {
				continue
			}
			found := false
			for _, v := range values {
				if v == field.value {
					found = true
				}
			}
			if !found {
				match = false
			}
		}
		if match {
			selected = append(selected, c)
		}
	}
	total := len(selected)
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	listOutput(w, 200, selected[start:end], int64(total), nil)
}
