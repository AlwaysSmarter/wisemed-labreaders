package siuidata

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type queryFilters map[string][]string

// Accept scalar, repeated, PHP-style [] / [0] and JSON-encoded arrays.
func parseFilters(q url.Values) (queryFilters, error) {
	result := queryFilters{}
	keys := make([]string, 0, len(q))
	for key := range q {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		name := key
		if i := strings.IndexByte(name, '['); i >= 0 {
			if !strings.HasSuffix(name, "]") {
				return nil, fmt.Errorf("invalid array filter %s", key)
			}
			index := name[i+1 : len(name)-1]
			if index != "" {
				if n, e := strconv.Atoi(index); e != nil || n < 0 {
					return nil, fmt.Errorf("invalid array filter %s", key)
				}
			}
			name = name[:i]
		}
		if name == "specialties" || name == "specialities" {
			name = "specialty"
		}
		for _, raw := range q[key] {
			values := []string{raw}
			if strings.HasPrefix(strings.TrimSpace(raw), "[") {
				var array []any
				if e := json.Unmarshal([]byte(raw), &array); e != nil {
					return nil, fmt.Errorf("invalid string array for %s", name)
				}
				values = []string{}
				for _, v := range array {
					value, ok := v.(string)
					if !ok {
						return nil, fmt.Errorf("invalid string array for %s", name)
					}
					values = append(values, value)
				}
			}
			for _, value := range values {
				if value == "" {
					continue
				}
				if len(value) > 512 || len(result[name]) >= 100 {
					return nil, fmt.Errorf("filter %s exceeds 100 values or 512 bytes per value", name)
				}
				exists := false
				for _, old := range result[name] {
					if old == value {
						exists = true
						break
					}
				}
				if !exists {
					result[name] = append(result[name], value)
				}
			}
		}
	}
	if len(result) > 64 {
		return nil, fmt.Errorf("too many filters")
	}
	return result, nil
}
func (f queryFilters) one(name string) (string, error) {
	values := f[name]
	if len(values) > 1 {
		return "", fmt.Errorf("%s must have a single value", name)
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return "", nil
}
func pagination(f queryFilters) (limit, start int, err error) {
	limit = 50
	for _, field := range []struct {
		name     string
		target   *int
		min, max int
	}{{"limit", &limit, 1, 1000}, {"start", &start, 0, 2147483647}} {
		raw, e := f.one(field.name)
		if e != nil {
			return 0, 0, e
		}
		if raw == "" {
			continue
		}
		v, e := strconv.Atoi(raw)
		if e != nil || v < field.min || v > field.max {
			return 0, 0, fmt.Errorf("%s must be %d..%d", field.name, field.min, field.max)
		}
		*field.target = v
	}
	return limit, start, nil
}

// textClauses preserves alternative matching and adds required fragments on the
// same SQL value. SQL columns are supplied only by the endpoint implementation.
func (f queryFilters) textClauses(args *[]any, dest *[]string, column, name string) {
	if values := f[name]; len(values) > 0 {
		patterns := make([]string, 0, len(values))
		for _, value := range values {
			patterns = append(patterns, likePattern(value))
		}
		*args = append(*args, patterns)
		*dest = append(*dest, fmt.Sprintf(column+" LIKE ANY($%d::text[])", len(*args)))
	}
	// Separate AND predicates allow PostgreSQL to use trigram indexes for each
	// required fragment; LIKE ALL is not eligible for the same index planning.
	for _, value := range f[name+"All"] {
		*args = append(*args, likePattern(value))
		*dest = append(*dest, fmt.Sprintf(column+" LIKE $%d", len(*args)))
	}
}

// Free-text search keeps its existing word semantics within each chip. q uses
// OR between chips; qAll uses AND. Every fragment is bound as a SQL parameter.
func (f queryFilters) searchClauses(args *[]any, dest *[]string, column string) error {
	for _, mode := range []struct{ suffix, operator string }{{"", " OR "}, {"All", " AND "}} {
		clauses := []string{}
		for _, value := range f["q"+mode.suffix] {
			words := strings.Fields(value)
			if len(words) > 8 {
				return fmt.Errorf("at most 8 search words per value")
			}
			and := []string{}
			for _, word := range words {
				if len([]rune(word)) < 2 {
					return fmt.Errorf("search words must have at least 2 characters")
				}
				*args = append(*args, likePattern(word))
				and = append(and, fmt.Sprintf(column+" LIKE $%d", len(*args)))
			}
			if len(and) > 0 {
				clauses = append(clauses, "("+strings.Join(and, " AND ")+")")
			}
		}
		if len(clauses) > 0 {
			*dest = append(*dest, "("+strings.Join(clauses, mode.operator)+")")
		}
	}
	return nil
}
