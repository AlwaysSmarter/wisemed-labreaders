package sqlite

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"sort"
	"strconv"
	"strings"

	coremodel "wisemed-labreaders/readersv3/modules/core/model"
)

const (
	transformFlagError   = "transformation_error"
	transformFlagMessage = "transformation_error_message"
)

type orderTransformationStore interface {
	ReapplyOrderTransformations(orderIDs []int64) error
}

type qcTransformationStore interface {
	ReapplyQCTransformations(recordIDs []int64) error
}

type transformedValues struct {
	ResultValue       string
	RawValue          string
	Interpreted       string
	SourceResultValue string
	SourceRawValue    string
	SourceInterpreted string
}

type formulaValue struct {
	kind string
	num  float64
	text string
	bool bool
}

type transformTrace struct {
	lines []string
}

func (t *transformTrace) addf(format string, args ...interface{}) {
	if t == nil {
		return
	}
	t.lines = append(t.lines, fmt.Sprintf(format, args...))
}

func (s *Store) transformationVerbose() int {
	if s == nil || s.verboseLevel == nil {
		return 1
	}
	return s.verboseLevel()
}

func (s *Store) transformationLogf(level int, format string, args ...interface{}) {
	if s == nil || s.logf == nil {
		return
	}
	if s.transformationVerbose() < level {
		return
	}
	s.logf(format, args...)
}

func (s *Store) ReapplyOrderTransformations(orderIDs []int64) error {
	orderIDs = uniqueInt64s(orderIDs)
	if len(orderIDs) == 0 {
		return nil
	}
	analytes, err := s.ListAnalytes()
	if err != nil {
		return err
	}
	byTag := map[string]coremodel.Analyte{}
	for _, item := range analytes {
		byTag[strings.TrimSpace(item.Tag)] = item
	}
	for _, orderID := range orderIDs {
		if err := s.reapplyOrderTransformations(orderID, byTag); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ReapplyQCTransformations(recordIDs []int64) error {
	recordIDs = uniqueInt64s(recordIDs)
	if len(recordIDs) == 0 {
		return nil
	}
	analytes, err := s.ListAnalytes()
	if err != nil {
		return err
	}
	byTag := map[string]coremodel.Analyte{}
	for _, item := range analytes {
		byTag[strings.TrimSpace(item.Tag)] = item
	}
	for _, recordID := range recordIDs {
		if err := s.reapplyQCTransformations(recordID, byTag); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) reapplyOrderTransformations(orderID int64, analytesByTag map[string]coremodel.Analyte) error {
	analyses, err := s.ListOrderAnalyses(orderID)
	if err != nil {
		return err
	}
	if len(analyses) == 0 {
		return nil
	}
	numericContext := buildOrderNumericContext(analyses, analytesByTag)
	for _, analysis := range analyses {
		traceEnabled := s.transformationVerbose() > 3
		var trace *transformTrace
		if traceEnabled {
			trace = &transformTrace{}
		}
		analyte, ok := analytesByTag[strings.TrimSpace(analysis.AnalyteTag)]
		if !ok || len(analyte.TransformRules) == 0 {
			flags := withoutTransformationFlags(analysis.Flags)
			current := baseTransformedValues(analysis.SourceResultValue, analysis.SourceRawValue, analysis.SourceInterpreted, analysis.ResultValue, analysis.RawValue, analysis.Interpreted)
			if trace != nil {
				trace.addf("no transform rules for analyte=%s", analysis.AnalyteTag)
			}
			if err := s.persistOrderTransformation(analysis, current, flags); err != nil {
				return err
			}
			s.logTransformationTrace("order", orderID, analysis.AnalyteTag, analysis.ID, trace)
			continue
		}
		current := baseTransformedValues(analysis.SourceResultValue, analysis.SourceRawValue, analysis.SourceInterpreted, analysis.ResultValue, analysis.RawValue, analysis.Interpreted)
		flags := withoutTransformationFlags(analysis.Flags)
		next, applyErr := applyAnalyteTransformRules(analyte, current, numericContext, trace)
		if applyErr != nil {
			flags[transformFlagError] = true
			flags[transformFlagMessage] = applyErr.Error()
			if trace != nil {
				trace.addf("error=%v", applyErr)
			}
		}
		if err := s.persistOrderTransformation(analysis, next, flags); err != nil {
			return err
		}
		s.logTransformationTrace("order", orderID, analysis.AnalyteTag, analysis.ID, trace)
	}
	return nil
}

func (s *Store) reapplyQCTransformations(recordID int64, analytesByTag map[string]coremodel.Analyte) error {
	analyses, err := s.ListQCAnalyses(recordID)
	if err != nil {
		return err
	}
	if len(analyses) == 0 {
		return nil
	}
	numericContext := buildQCNumericContext(analyses, analytesByTag)
	for _, analysis := range analyses {
		traceEnabled := s.transformationVerbose() > 3
		var trace *transformTrace
		if traceEnabled {
			trace = &transformTrace{}
		}
		analyte, ok := analytesByTag[strings.TrimSpace(analysis.AnalyteTag)]
		if !ok || len(analyte.TransformRules) == 0 {
			flags := withoutTransformationFlags(analysis.Flags)
			current := baseTransformedValues(analysis.SourceResultValue, analysis.SourceRawValue, analysis.SourceInterpreted, analysis.ResultValue, analysis.RawValue, analysis.Interpreted)
			if trace != nil {
				trace.addf("no transform rules for analyte=%s", analysis.AnalyteTag)
			}
			if err := s.persistQCTransformation(analysis, current, flags); err != nil {
				return err
			}
			s.logTransformationTrace("qc", recordID, analysis.AnalyteTag, analysis.ID, trace)
			continue
		}
		current := baseTransformedValues(analysis.SourceResultValue, analysis.SourceRawValue, analysis.SourceInterpreted, analysis.ResultValue, analysis.RawValue, analysis.Interpreted)
		flags := withoutTransformationFlags(analysis.Flags)
		next, applyErr := applyAnalyteTransformRules(analyte, current, numericContext, trace)
		if applyErr != nil {
			flags[transformFlagError] = true
			flags[transformFlagMessage] = applyErr.Error()
			if trace != nil {
				trace.addf("error=%v", applyErr)
			}
		}
		if err := s.persistQCTransformation(analysis, next, flags); err != nil {
			return err
		}
		s.logTransformationTrace("qc", recordID, analysis.AnalyteTag, analysis.ID, trace)
	}
	return nil
}

func (s *Store) logTransformationTrace(scope string, ownerID int64, analyteTag string, analysisID int64, trace *transformTrace) {
	if trace == nil || len(trace.lines) == 0 {
		return
	}
	for _, line := range trace.lines {
		s.transformationLogf(4, "transform %s owner=%d analysis_id=%d analyte=%s %s", scope, ownerID, analysisID, analyteTag, line)
	}
}

func (s *Store) persistOrderTransformation(analysis coremodel.OrderAnalysis, values transformedValues, flags map[string]interface{}) error {
	updated, err := s.SaveOrderAnalysis(coremodel.OrderAnalysis{
		ID:                analysis.ID,
		OrderID:           analysis.OrderID,
		AnalyteID:         analysis.AnalyteID,
		AnalyteTag:        analysis.AnalyteTag,
		AnalyteName:       analysis.AnalyteName,
		WiseMEDSMID:       analysis.WiseMEDSMID,
		WiseMEDFSMID:      analysis.WiseMEDFSMID,
		Status:            analysis.Status,
		DefaultResultID:   analysis.DefaultResultID,
		ResultValue:       values.ResultValue,
		RawValue:          values.RawValue,
		Interpreted:       values.Interpreted,
		SourceResultValue: values.SourceResultValue,
		SourceRawValue:    values.SourceRawValue,
		SourceInterpreted: values.SourceInterpreted,
		Unit:              analysis.Unit,
		SourceFile:        analysis.SourceFile,
		Flags:             flags,
		Meta:              analysis.Meta,
	})
	if err != nil {
		return err
	}
	resultID := updated.DefaultResultID
	if resultID <= 0 {
		results, listErr := s.ListResultsForAnalysis(analysis.ID)
		if listErr == nil && len(results) > 0 {
			resultID = results[0].ID
		}
	}
	if resultID > 0 {
		return s.updateOrderAnalysisResultFields(resultID, values, flags)
	}
	return nil
}

func (s *Store) persistQCTransformation(analysis coremodel.QCAnalysis, values transformedValues, flags map[string]interface{}) error {
	_, err := s.UpsertQCAnalysis(coremodel.QCAnalysis{
		ID:                 analysis.ID,
		QCRecordID:         analysis.QCRecordID,
		AnalyteID:          analysis.AnalyteID,
		AnalyteTag:         analysis.AnalyteTag,
		AnalyteName:        analysis.AnalyteName,
		ControlLevel:       analysis.ControlLevel,
		LotNo:              analysis.LotNo,
		AnalyteDescription: analysis.AnalyteDescription,
		Status:             analysis.Status,
		DefaultResultID:    analysis.DefaultResultID,
		ResultValue:        values.ResultValue,
		RawValue:           values.RawValue,
		Interpreted:        values.Interpreted,
		SourceResultValue:  values.SourceResultValue,
		SourceRawValue:     values.SourceRawValue,
		SourceInterpreted:  values.SourceInterpreted,
		Unit:               analysis.Unit,
		SourceFile:         analysis.SourceFile,
		Flags:              flags,
		Meta:               analysis.Meta,
	})
	return err
}

func (s *Store) updateOrderAnalysisResultFields(resultID int64, values transformedValues, flags map[string]interface{}) error {
	flagsJSON, _ := json.Marshal(metaOrEmpty(flags))
	_, err := s.db.Exec(`update order_analysis_results set result_value=?,raw_value=?,interpreted_value=?,source_result_value=?,source_raw_value=?,source_interpreted_value=?,flags_json=? where id = ?`,
		values.ResultValue, values.RawValue, values.Interpreted, values.SourceResultValue, values.SourceRawValue, values.SourceInterpreted, string(flagsJSON), resultID)
	return err
}

func buildOrderNumericContext(analyses []coremodel.OrderAnalysis, analytesByTag map[string]coremodel.Analyte) map[string]float64 {
	out := map[string]float64{}
	for _, analysis := range analyses {
		analyte, ok := analytesByTag[strings.TrimSpace(analysis.AnalyteTag)]
		if !ok {
			continue
		}
		code := normalizeFormulaCode(analyte.ResultFormulaCode)
		if code == "" {
			continue
		}
		value, ok := firstNumericValue(analysis.SourceRawValue, analysis.SourceResultValue, analysis.RawValue, analysis.ResultValue)
		if !ok {
			continue
		}
		out["RES_"+code] = value
	}
	return out
}

func buildQCNumericContext(analyses []coremodel.QCAnalysis, analytesByTag map[string]coremodel.Analyte) map[string]float64 {
	out := map[string]float64{}
	for _, analysis := range analyses {
		analyte, ok := analytesByTag[strings.TrimSpace(analysis.AnalyteTag)]
		if !ok {
			continue
		}
		code := normalizeFormulaCode(analyte.ResultFormulaCode)
		if code == "" {
			continue
		}
		value, ok := firstNumericValue(analysis.SourceRawValue, analysis.SourceResultValue, analysis.RawValue, analysis.ResultValue)
		if !ok {
			continue
		}
		out["RES_"+code] = value
	}
	return out
}

func applyAnalyteTransformRules(analyte coremodel.Analyte, current transformedValues, numericContext map[string]float64, trace *transformTrace) (transformedValues, error) {
	rules := append([]coremodel.TransformRule(nil), analyte.TransformRules...)
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Order == rules[j].Order {
			return rules[i].ID < rules[j].ID
		}
		return rules[i].Order < rules[j].Order
	})
	if trace != nil {
		trace.addf("start source_result=%q source_raw=%q source_interpreted=%q current_result=%q current_raw=%q current_interpreted=%q",
			current.SourceResultValue, current.SourceRawValue, current.SourceInterpreted, current.ResultValue, current.RawValue, current.Interpreted)
		trace.addf("numeric_context=%s", formatNumericContext(numericContext))
	}
	var applyErr error
	for index, rule := range rules {
		if !rule.Enabled {
			if trace != nil {
				trace.addf("rule[%d] id=%q skipped because disabled", index+1, rule.ID)
			}
			continue
		}
		applied := false
		if trace != nil {
			trace.addf("rule[%d] id=%q type=%q source=%q target=%q mode=%q stop_after_apply=%t",
				index+1, rule.ID, rule.Type, rule.SourceField, rule.TargetField, rule.TargetMode, rule.StopAfterApply)
		}
		switch strings.ToLower(strings.TrimSpace(rule.Type)) {
		case "value_map", "value-map", "map":
			var ok bool
			current, ok = applyValueMapRule(current, rule, trace)
			applied = ok
		case "formula":
			next, ok, err := applyFormulaRule(analyte, current, rule, numericContext, trace)
			if err != nil {
				applyErr = err
				break
			}
			current = next
			applied = ok
		default:
			applyErr = fmt.Errorf("tip regula necunoscut: %s", rule.Type)
		}
		if applyErr != nil {
			break
		}
		if trace != nil {
			trace.addf("rule[%d] applied=%t result_value=%q raw_value=%q interpreted=%q", index+1, applied, current.ResultValue, current.RawValue, current.Interpreted)
		}
		if applied && rule.StopAfterApply {
			if trace != nil {
				trace.addf("rule[%d] requested stop after apply", index+1)
			}
			break
		}
	}
	return current, applyErr
}

func applyValueMapRule(current transformedValues, rule coremodel.TransformRule, trace *transformTrace) (transformedValues, bool) {
	source := ruleSourceValue(current, rule.SourceField)
	match := strings.TrimSpace(rule.MatchValue)
	candidate := strings.TrimSpace(source)
	if trace != nil {
		trace.addf("value_map source_value=%q match=%q ignore_case=%t output=%q", candidate, match, rule.MatchIgnoreCase, rule.OutputValue)
	}
	if rule.MatchIgnoreCase {
		if !strings.EqualFold(candidate, match) {
			return current, false
		}
	} else if candidate != match {
		return current, false
	}
	current = applyTargetValue(current, rule.TargetField, rule.TargetMode, rule.OutputValue)
	return current, true
}

func applyFormulaRule(analyte coremodel.Analyte, current transformedValues, rule coremodel.TransformRule, numericContext map[string]float64, trace *transformTrace) (transformedValues, bool, error) {
	formula := normalizeFormulaSyntax(strings.TrimSpace(rule.Formula))
	if formula == "" {
		return current, false, nil
	}
	vars := map[string]float64{}
	for key, value := range numericContext {
		vars[key] = value
	}
	if value, ok := firstNumericValue(current.SourceResultValue, current.ResultValue); ok {
		vars["SRC_RESULT"] = value
	}
	if value, ok := firstNumericValue(current.SourceRawValue, current.RawValue); ok {
		vars["SRC_RAW"] = value
		vars["RAW"] = value
	}
	if value, ok := firstNumericValue(current.ResultValue); ok {
		vars["CUR_RESULT"] = value
	}
	if value, ok := firstNumericValue(current.RawValue); ok {
		vars["CUR_RAW"] = value
		vars["CUR"] = value
	}
	if trace != nil {
		trace.addf("formula=%q vars=%s", formula, formatNumericContext(vars))
	}
	result, err := evalFormula(formula, vars)
	if err != nil {
		return current, false, err
	}
	output := formatFormulaValue(result, analyte.ResultFormatting)
	if trace != nil {
		trace.addf("formula_result=%v formatted_output=%q", result, output)
	}
	current = applyTargetValue(current, rule.TargetField, rule.TargetMode, output)
	return current, true, nil
}

func applyTargetValue(current transformedValues, targetField, targetMode, output string) transformedValues {
	target := normalizeTargetField(targetField)
	mode := strings.ToLower(strings.TrimSpace(targetMode))
	switch target {
	case "interpreted":
		if mode == "append" || mode == "add" {
			current.Interpreted = appendInterpreted(current.Interpreted, output)
		} else {
			current.Interpreted = output
		}
	case "raw_value":
		current.RawValue = output
	default:
		current.ResultValue = output
	}
	return current
}

func ruleSourceValue(current transformedValues, sourceField string) string {
	switch strings.ToLower(strings.TrimSpace(sourceField)) {
	case "source_raw", "analyzer_raw", "raw_source":
		return current.SourceRawValue
	case "source_interpreted", "analyzer_interpreted":
		return current.SourceInterpreted
	case "current_raw", "raw_value", "numeric":
		return current.RawValue
	case "current_interpreted", "interpreted":
		return current.Interpreted
	case "source_result", "analyzer_result", "":
		return current.SourceResultValue
	default:
		return current.ResultValue
	}
}

func normalizeTargetField(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "raw", "raw_value", "quantitative", "numeric":
		return "raw_value"
	case "interpreted", "interpretation", "interpreted_value":
		return "interpreted"
	default:
		return "result_value"
	}
}

func appendInterpreted(existing, value string) string {
	existing = strings.TrimSpace(existing)
	value = strings.TrimSpace(value)
	if existing == "" {
		return value
	}
	if value == "" {
		return existing
	}
	return existing + "\n" + value
}

func withoutTransformationFlags(flags map[string]interface{}) map[string]interface{} {
	out := mergeMeta(flags, nil)
	delete(out, transformFlagError)
	delete(out, transformFlagMessage)
	return out
}

func baseTransformedValues(sourceResult, sourceRaw, sourceInterpreted, currentResult, currentRaw, currentInterpreted string) transformedValues {
	sourceResult = firstNonEmptyText(sourceResult, currentResult)
	sourceRaw = firstNonEmptyText(sourceRaw, currentRaw)
	sourceInterpreted = firstNonEmptyText(sourceInterpreted, currentInterpreted)
	return transformedValues{
		ResultValue:       sourceResult,
		RawValue:          sourceRaw,
		Interpreted:       sourceInterpreted,
		SourceResultValue: sourceResult,
		SourceRawValue:    sourceRaw,
		SourceInterpreted: sourceInterpreted,
	}
}

func firstNonEmptyText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeFormulaCode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range strings.ToUpper(value) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		if b.Len() == 0 || b.String()[b.Len()-1] == '_' {
			continue
		}
		b.WriteByte('_')
	}
	return strings.Trim(b.String(), "_")
}

func firstNumericValue(values ...string) (float64, bool) {
	for _, value := range values {
		candidate := strings.TrimSpace(strings.ReplaceAll(value, ",", "."))
		if candidate == "" {
			continue
		}
		if parsed, err := strconv.ParseFloat(candidate, 64); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func formatFormulaResult(value float64, formatting string) string {
	switch strings.ToLower(strings.TrimSpace(formatting)) {
	case "numeric", "raw", "text":
		return trimFloat(value, -1)
	case "decimal_1":
		return trimFloat(value, 1)
	case "decimal_2":
		return trimFloat(value, 2)
	case "decimal_3":
		return trimFloat(value, 3)
	case "decimal_4":
		return trimFloat(value, 4)
	default:
		return trimFloat(value, -1)
	}
}

func formatFormulaValue(value formulaValue, formatting string) string {
	switch value.kind {
	case "text":
		return strings.TrimSpace(value.text)
	case "bool":
		if value.bool {
			return "true"
		}
		return "false"
	default:
		return formatFormulaResult(value.num, formatting)
	}
}

func trimFloat(value float64, decimals int) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return ""
	}
	if decimals >= 0 {
		return strconv.FormatFloat(value, 'f', decimals, 64)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func formatNumericContext(values map[string]float64) string {
	if len(values) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, trimFloat(values[key], -1)))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func evalFormula(formula string, vars map[string]float64) (formulaValue, error) {
	expr, err := parser.ParseExpr(strings.TrimSpace(formula))
	if err != nil {
		return formulaValue{}, err
	}
	return evalExprNode(expr, vars)
}

func evalExprNode(expr ast.Expr, vars map[string]float64) (formulaValue, error) {
	switch node := expr.(type) {
	case *ast.BasicLit:
		switch node.Kind {
		case token.FLOAT, token.INT:
			value, err := strconv.ParseFloat(node.Value, 64)
			if err != nil {
				return formulaValue{}, err
			}
			return formulaValue{kind: "number", num: value}, nil
		case token.STRING:
			value, err := strconv.Unquote(node.Value)
			if err != nil {
				return formulaValue{}, err
			}
			return formulaValue{kind: "text", text: value}, nil
		default:
			return formulaValue{}, fmt.Errorf("literal nesuportat: %s", node.Value)
		}
	case *ast.ParenExpr:
		return evalExprNode(node.X, vars)
	case *ast.Ident:
		value, ok := vars[strings.TrimSpace(node.Name)]
		if !ok {
			return formulaValue{}, fmt.Errorf("variabila lipsa: %s", node.Name)
		}
		return formulaValue{kind: "number", num: value}, nil
	case *ast.UnaryExpr:
		value, err := evalExprNode(node.X, vars)
		if err != nil {
			return formulaValue{}, err
		}
		number, err := formulaNumber(value)
		if err != nil {
			return formulaValue{}, err
		}
		switch node.Op {
		case token.ADD:
			return formulaValue{kind: "number", num: number}, nil
		case token.SUB:
			return formulaValue{kind: "number", num: -number}, nil
		default:
			return formulaValue{}, fmt.Errorf("operator unar nesuportat: %s", node.Op)
		}
	case *ast.BinaryExpr:
		left, err := evalExprNode(node.X, vars)
		if err != nil {
			return formulaValue{}, err
		}
		right, err := evalExprNode(node.Y, vars)
		if err != nil {
			return formulaValue{}, err
		}
		switch node.Op {
		case token.ADD:
			lv, err := formulaNumber(left)
			if err != nil {
				return formulaValue{}, err
			}
			rv, err := formulaNumber(right)
			if err != nil {
				return formulaValue{}, err
			}
			return formulaValue{kind: "number", num: lv + rv}, nil
		case token.SUB:
			lv, err := formulaNumber(left)
			if err != nil {
				return formulaValue{}, err
			}
			rv, err := formulaNumber(right)
			if err != nil {
				return formulaValue{}, err
			}
			return formulaValue{kind: "number", num: lv - rv}, nil
		case token.MUL:
			lv, err := formulaNumber(left)
			if err != nil {
				return formulaValue{}, err
			}
			rv, err := formulaNumber(right)
			if err != nil {
				return formulaValue{}, err
			}
			return formulaValue{kind: "number", num: lv * rv}, nil
		case token.QUO:
			lv, err := formulaNumber(left)
			if err != nil {
				return formulaValue{}, err
			}
			rv, err := formulaNumber(right)
			if err != nil {
				return formulaValue{}, err
			}
			if rv == 0 {
				return formulaValue{}, errors.New("impartire la zero")
			}
			return formulaValue{kind: "number", num: lv / rv}, nil
		case token.GTR, token.GEQ, token.LSS, token.LEQ, token.EQL, token.NEQ:
			lv, err := formulaNumber(left)
			if err != nil {
				return formulaValue{}, err
			}
			rv, err := formulaNumber(right)
			if err != nil {
				return formulaValue{}, err
			}
			out := false
			switch node.Op {
			case token.GTR:
				out = lv > rv
			case token.GEQ:
				out = lv >= rv
			case token.LSS:
				out = lv < rv
			case token.LEQ:
				out = lv <= rv
			case token.EQL:
				out = lv == rv
			case token.NEQ:
				out = lv != rv
			}
			return formulaValue{kind: "bool", bool: out}, nil
		default:
			return formulaValue{}, fmt.Errorf("operator nesuportat: %s", node.Op)
		}
	case *ast.CallExpr:
		fn, ok := node.Fun.(*ast.Ident)
		if !ok {
			return formulaValue{}, fmt.Errorf("apel functie nesuportat")
		}
		name := strings.ToUpper(strings.TrimSpace(fn.Name))
		switch name {
		case "IF", "IIF":
			if len(node.Args) != 3 {
				return formulaValue{}, fmt.Errorf("IF necesita 3 parametri")
			}
			cond, err := evalExprNode(node.Args[0], vars)
			if err != nil {
				return formulaValue{}, err
			}
			okCond, err := formulaBool(cond)
			if err != nil {
				return formulaValue{}, err
			}
			if okCond {
				return evalExprNode(node.Args[1], vars)
			}
			return evalExprNode(node.Args[2], vars)
		case "SQRT":
			args, err := evalFormulaNumbers(node.Args, vars)
			if err != nil {
				return formulaValue{}, err
			}
			if len(args) != 1 {
				return formulaValue{}, fmt.Errorf("SQRT necesita 1 parametru")
			}
			if args[0] < 0 {
				return formulaValue{}, errors.New("SQRT nu accepta valori negative")
			}
			return formulaValue{kind: "number", num: math.Sqrt(args[0])}, nil
		case "ABS":
			args, err := evalFormulaNumbers(node.Args, vars)
			if err != nil {
				return formulaValue{}, err
			}
			if len(args) != 1 {
				return formulaValue{}, fmt.Errorf("ABS necesita 1 parametru")
			}
			return formulaValue{kind: "number", num: math.Abs(args[0])}, nil
		case "ROUND":
			args, err := evalFormulaNumbers(node.Args, vars)
			if err != nil {
				return formulaValue{}, err
			}
			if len(args) < 1 || len(args) > 2 {
				return formulaValue{}, fmt.Errorf("ROUND necesita 1 sau 2 parametri")
			}
			decimals := 0.0
			if len(args) == 2 {
				decimals = args[1]
			}
			factor := math.Pow(10, math.Round(decimals))
			return formulaValue{kind: "number", num: math.Round(args[0]*factor) / factor}, nil
		case "POW", "POWER":
			args, err := evalFormulaNumbers(node.Args, vars)
			if err != nil {
				return formulaValue{}, err
			}
			if len(args) != 2 {
				return formulaValue{}, fmt.Errorf("%s necesita 2 parametri", name)
			}
			return formulaValue{kind: "number", num: math.Pow(args[0], args[1])}, nil
		case "MIN":
			args, err := evalFormulaNumbers(node.Args, vars)
			if err != nil {
				return formulaValue{}, err
			}
			if len(args) == 0 {
				return formulaValue{}, fmt.Errorf("MIN necesita cel putin 1 parametru")
			}
			out := args[0]
			for _, value := range args[1:] {
				out = math.Min(out, value)
			}
			return formulaValue{kind: "number", num: out}, nil
		case "MAX":
			args, err := evalFormulaNumbers(node.Args, vars)
			if err != nil {
				return formulaValue{}, err
			}
			if len(args) == 0 {
				return formulaValue{}, fmt.Errorf("MAX necesita cel putin 1 parametru")
			}
			out := args[0]
			for _, value := range args[1:] {
				out = math.Max(out, value)
			}
			return formulaValue{kind: "number", num: out}, nil
		case "SUM":
			args, err := evalFormulaNumbers(node.Args, vars)
			if err != nil {
				return formulaValue{}, err
			}
			if len(args) == 0 {
				return formulaValue{}, fmt.Errorf("SUM necesita cel putin 1 parametru")
			}
			out := 0.0
			for _, value := range args {
				out += value
			}
			return formulaValue{kind: "number", num: out}, nil
		case "AVG", "AVERAGE":
			args, err := evalFormulaNumbers(node.Args, vars)
			if err != nil {
				return formulaValue{}, err
			}
			if len(args) == 0 {
				return formulaValue{}, fmt.Errorf("%s necesita cel putin 1 parametru", name)
			}
			out := 0.0
			for _, value := range args {
				out += value
			}
			return formulaValue{kind: "number", num: out / float64(len(args))}, nil
		default:
			return formulaValue{}, fmt.Errorf("functie nesuportata: %s", fn.Name)
		}
	default:
		return formulaValue{}, fmt.Errorf("expresie nesuportata")
	}
}

func evalFormulaNumbers(args []ast.Expr, vars map[string]float64) ([]float64, error) {
	values := make([]float64, 0, len(args))
	for _, arg := range args {
		value, err := evalExprNode(arg, vars)
		if err != nil {
			return nil, err
		}
		number, err := formulaNumber(value)
		if err != nil {
			return nil, err
		}
		values = append(values, number)
	}
	return values, nil
}

func formulaNumber(value formulaValue) (float64, error) {
	switch value.kind {
	case "number", "":
		return value.num, nil
	case "bool":
		if value.bool {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("valoare nenumerica in formula")
	}
}

func formulaBool(value formulaValue) (bool, error) {
	switch value.kind {
	case "bool":
		return value.bool, nil
	case "number", "":
		return value.num != 0, nil
	default:
		return false, fmt.Errorf("conditie invalida in formula")
	}
}

func normalizeFormulaSyntax(formula string) string {
	formula = strings.TrimSpace(formula)
	if formula == "" {
		return ""
	}
	formula = convertSingleQuotedStrings(formula)
	if strings.Contains(formula, ":") && !strings.Contains(formula, "IF(") && !strings.Contains(formula, "IIF(") {
		if converted, ok := convertLegacyConditionalFormula(formula); ok {
			return converted
		}
	}
	return formula
}

func convertSingleQuotedStrings(formula string) string {
	var b strings.Builder
	for i := 0; i < len(formula); i++ {
		if formula[i] != '\'' {
			b.WriteByte(formula[i])
			continue
		}
		j := i + 1
		for j < len(formula) && formula[j] != '\'' {
			j++
		}
		if j >= len(formula) {
			b.WriteByte(formula[i])
			continue
		}
		b.WriteString(strconv.Quote(formula[i+1 : j]))
		i = j
	}
	return b.String()
}

func convertLegacyConditionalFormula(formula string) (string, bool) {
	trimmed := strings.TrimSpace(formula)
	if strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	colon := topLevelIndex(trimmed, ':')
	if colon <= 0 || colon >= len(trimmed)-1 {
		return "", false
	}
	left := strings.TrimSpace(trimmed[:colon])
	right := strings.TrimSpace(trimmed[colon+1:])
	trueStart := lastTopLevelStringStart(left)
	if trueStart <= 0 {
		return "", false
	}
	condition := strings.TrimSpace(left[:trueStart])
	trueExpr := strings.TrimSpace(left[trueStart:])
	if condition == "" || trueExpr == "" || right == "" {
		return "", false
	}
	return fmt.Sprintf("IF(%s, %s, %s)", condition, trueExpr, right), true
}

func topLevelIndex(value string, needle byte) int {
	depth := 0
	inString := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch == '"' && (i == 0 || value[i-1] != '\\') {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if ch == needle && depth == 0 {
				return i
			}
		}
	}
	return -1
}

func lastTopLevelStringStart(value string) int {
	depth := 0
	inString := false
	start := -1
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch == '"' && (i == 0 || value[i-1] != '\\') {
			if !inString && depth == 0 {
				start = i
			}
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
	}
	return start
}

func uniqueInt64s(values []int64) []int64 {
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
