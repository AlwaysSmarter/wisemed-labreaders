package siuidata

import (
	"context"
	"net/http"
	"strings"
)

type targetProfile struct {
	CUI, Day    string
	Specialties []string
}
type targetProfileKey struct{}

func targetedPath(path string) (string, string, bool) {
	route := strings.TrimPrefix(path, prefix)
	simple := false
	if strings.HasPrefix(route, "targeted_services/") {
		route = strings.TrimPrefix(route, "targeted_services/")
	} else if strings.HasPrefix(route, "targeted-simple-services/") {
		route = strings.TrimPrefix(route, "targeted-simple-services/")
		simple = true
	} else {
		return "", "", false
	}
	if strings.ContainsAny(route, "/\\\x00") || route == "" {
		return "", "", false
	}
	cui, e := normalizeCUI(route)
	if e != nil {
		return "", "", false
	}
	endpoint := "services"
	if simple {
		endpoint = "simple-services"
	}
	return cui, endpoint, true
}
func (m *Module) targetedHTTP(w http.ResponseWriter, r *http.Request) {
	cui, route, ok := targetedPath(r.URL.Path)
	if !ok {
		failure(w, 400, "invalid targeted services path")
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		failure(w, 405, "GET or HEAD required")
		return
	}
	if c, ok := r.Context().Value(identityKey{}).(*claims); ok {
		if c.CUI != "" && c.CUI != cui {
			failure(w, 403, "CUI access denied")
			return
		}
		if c.CUI == "" && !c.Admin {
			failure(w, 403, "global administrator JWT required")
			return
		}
	} else if !m.internalBrowser(r) {
		failure(w, 403, "CUI JWT or local administrator browser session required")
		return
	}
	f, e := parseFilters(r.URL.Query())
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	day, e := f.one("atDate")
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	legacy, e := f.one("date")
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	if day != "" && legacy != "" {
		failure(w, 400, "use atDate only")
		return
	}
	if day == "" {
		day = legacy
	}
	day, e = queryDay(day, m.loc)
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	for _, name := range []string{"type", "partnerCategory"} {
		for _, v := range f[name] {
			if strings.ToUpper(v) != "CLIN" {
				failure(w, 400, "targeted services currently require CLIN")
				return
			}
		}
	}
	allowed := map[string]bool{"atDate": true, "date": true, "type": true, "partnerCategory": true, "code": true, "q": true, "qAll": true, "name": true, "nameAll": true, "kind": true, "specialty": true, "specialtyName": true, "specialtyNameAll": true, "package": true, "group": true, "isExam": true, "opRoom": true, "isConnectedService": true, "limit": true, "start": true}
	for key := range f {
		if !allowed[key] && !strings.HasPrefix(key, "attr.") {
			failure(w, 400, "unknown targeted service filter: "+key)
			return
		}
	}
	clone := r.Clone(context.WithValue(r.Context(), targetProfileKey{}, &targetProfile{CUI: cui, Day: day}))
	u := *r.URL
	clone.URL = &u
	q := r.URL.Query()
	for key := range q {
		base := strings.SplitN(key, "[", 2)[0]
		if base == "atDate" || base == "date" || base == "type" || base == "partnerCategory" {
			q.Del(key)
		}
	}
	q.Set("type", "CLIN")
	clone.URL.RawQuery = q.Encode()
	m.queryHTTP(w, clone, route)
}
