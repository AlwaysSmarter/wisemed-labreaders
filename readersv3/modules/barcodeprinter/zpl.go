package barcodeprinter

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type LayoutSettings struct {
	PrinterResolution     int
	LabelWidth            int
	BarcodeType           string
	BarcodeCodeX          int
	BarcodeCodeY          int
	BarcodeWidth          string
	BarcodeWideNarrow     string
	BarcodeHeight         int
	BarcodeOptOrientation string
	BarcodeOptCheckDigit  string
	BarcodeOptInterpLine  string
	BarcodeOptInterpAbove string
	BarcodeTxtX           int
	BarcodeTxtY           int
	BarcodeTxtFont        string
	BarcodeTxtOrientation string
	BarcodeTxtHeight      int
	BarcodeTxtWidth       int
	PatientNameX          int
	PatientNameY          int
	PatientNameFont       string
	PatientNameOrient     string
	PatientNameHeight     int
	PatientNameWidth      int
	TubeCodeX             int
	TubeCodeY             int
	TubeCodeFont          string
	TubeCodeOrientation   string
	TubeCodeHeight        int
	TubeCodeWidth         int
	TubeCodeBoxWidth      int
	TubeCodeBoxHeight     int
	TubeCodeBoxThickness  int
	TubeCodeBoxColor      string
	TubeCodeBoxRadius     int
}

type ZPLPrinter struct {
	Settings    LayoutSettings
	Barcode     string
	PatientName string
	TubeCode    string
}

func newZPLPrinterFromParams(params map[string]string) (*ZPLPrinter, error) {
	barcodeValue := strings.TrimSpace(firstNonEmpty(params["bc"], params["fileid"], params["code"]))
	if barcodeValue == "" {
		return nil, fmt.Errorf("unknown barcode to print")
	}
	tubeCode := strings.TrimSpace(firstNonEmpty(params["tc"], params["vc"]))
	if len(tubeCode) > 1 {
		tubeCode = tubeCode[:1]
	}
	return &ZPLPrinter{
		Settings:    parseLayoutSettings(params),
		Barcode:     barcodeValue,
		PatientName: strings.TrimSpace(firstNonEmpty(params["pn"], params["name"])),
		TubeCode:    tubeCode,
	}, nil
}

func parseLayoutSettings(params map[string]string) LayoutSettings {
	res := intParam(params, "bcp_resolution", "othercfg_printer_resolution", 200, false, 0)
	if res <= 0 {
		res = 200
	}
	return LayoutSettings{
		PrinterResolution:     res,
		LabelWidth:            intParam(params, "bc_label_width", "othercfg_label_width", 50, true, res),
		BarcodeType:           strParam(params, "bc_bctype", "othercfg_printer_barcode", "B3"),
		BarcodeCodeX:          intParam(params, "bc_bcx", "othercfg_print_bcodex", 5, true, res),
		BarcodeCodeY:          intParam(params, "bc_bcy", "othercfg_print_bcodey", 5, true, res),
		BarcodeWidth:          strParam(params, "bc_opt_w", "othercfg_print_bcodeopt_w", "2"),
		BarcodeWideNarrow:     strParam(params, "bc_widenarrowratio", "othercfg_bc_widenarowr", "3.0"),
		BarcodeHeight:         intParam(params, "bc_opt_h", "othercfg_print_bcodeopt_h", 50, false, 0),
		BarcodeOptOrientation: strParam(params, "bc_opt_o", "othercfg_print_bcodeopt_o", "N"),
		BarcodeOptCheckDigit:  strParam(params, "bc_opt_e", "othercfg_print_bcodeopt_e", "N"),
		BarcodeOptInterpLine:  strParam(params, "bc_opt_f", "othercfg_print_bcodeopt_f", "N"),
		BarcodeOptInterpAbove: strParam(params, "bc_opt_g", "othercfg_print_bcodeopt_g", "N"),
		BarcodeTxtX:           intParam(params, "bc_bccodex", "othercfg_print_bcodetxtx", 80, true, res),
		BarcodeTxtY:           intParam(params, "bc_bccodey", "othercfg_print_bcodetxty", 40, true, res),
		BarcodeTxtFont:        strParam(params, "bc_bccodef", "othercfg_print_bcodetxtf", "D"),
		BarcodeTxtOrientation: strParam(params, "bc_bccodeo", "othercfg_print_bcodetxto", "N"),
		BarcodeTxtHeight:      intParam(params, "bc_bccodeh", "othercfg_print_bcodetxth", 6, false, 0),
		BarcodeTxtWidth:       intParam(params, "bc_bccodew", "othercfg_print_bcodetxtw", 6, false, 0),
		PatientNameX:          intParam(params, "bc_patx", "othercfg_print_namex", 5, true, res),
		PatientNameY:          intParam(params, "bc_paty", "othercfg_print_namey", 95, true, res),
		PatientNameFont:       strParam(params, "bc_patf", "othercfg_print_namef", "B"),
		PatientNameOrient:     strParam(params, "bc_pato", "othercfg_print_nameo", "N"),
		PatientNameHeight:     intParam(params, "bc_path", "othercfg_print_nameh", 6, false, 0),
		PatientNameWidth:      intParam(params, "bc_patw", "othercfg_print_namew", 6, false, 0),
		TubeCodeX:             intParam(params, "bc_tubcodx", "othercfg_print_tubecodex", 40.64, true, res),
		TubeCodeY:             intParam(params, "bc_tubcody", "othercfg_print_tubecodey", 22.86, true, res),
		TubeCodeFont:          strParam(params, "bc_tubcodf", "othercfg_print_tubecodef", "B"),
		TubeCodeOrientation:   strParam(params, "bc_tubcodo", "othercfg_print_tubecodeo", "N"),
		TubeCodeHeight:        intParam(params, "bc_tubcodh", "othercfg_print_tubecodeh", 6, false, 0),
		TubeCodeWidth:         intParam(params, "bc_tubcodw", "othercfg_print_tubecodew", 6, false, 0),
		TubeCodeBoxWidth:      intParam(params, "bc_tubcod_boxw", "othercfg_print_tubecode_boxw", 3.81, true, res),
		TubeCodeBoxHeight:     intParam(params, "bc_tubcod_boxh", "othercfg_print_tubecode_boxh", 3.81, true, res),
		TubeCodeBoxThickness:  intParam(params, "bc_tubcod_boxt", "othercfg_print_tubecode_boxt", 1, false, 0),
		TubeCodeBoxColor:      strParam(params, "bc_tubcod_boxc", "othercfg_print_tubecode_boxc", "B"),
		TubeCodeBoxRadius:     intParam(params, "bc_tubcod_boxr", "othercfg_print_tubecode_boxr", 4, false, 0),
	}
}

func (p *ZPLPrinter) RenderZPL() string {
	var b strings.Builder
	b.WriteString("^XA\n")
	b.WriteString(fmt.Sprintf("^PW%d\n", p.Settings.LabelWidth))
	b.WriteString(fmt.Sprintf("^FO%d,%d^BY%s,%s,%d^%s%s,%s,%d,%s,%s^FD%s^FS\n",
		p.Settings.BarcodeCodeX,
		p.Settings.BarcodeCodeY,
		p.Settings.BarcodeWidth,
		p.Settings.BarcodeWideNarrow,
		p.Settings.BarcodeHeight,
		p.Settings.BarcodeType,
		p.Settings.BarcodeOptOrientation,
		p.Settings.BarcodeOptCheckDigit,
		p.Settings.BarcodeHeight,
		p.Settings.BarcodeOptInterpLine,
		p.Settings.BarcodeOptInterpAbove,
		p.Barcode,
	))
	b.WriteString(fmt.Sprintf("^FO%d,%d^A%s%s,%d,%d^FD%s^FS\n",
		p.Settings.BarcodeTxtX,
		p.Settings.BarcodeTxtY,
		p.Settings.BarcodeTxtFont,
		p.Settings.BarcodeTxtOrientation,
		p.Settings.BarcodeTxtHeight,
		p.Settings.BarcodeTxtWidth,
		p.Barcode,
	))
	if strings.TrimSpace(p.PatientName) != "" {
		b.WriteString(fmt.Sprintf("^FO%d,%d^A%s%s,%d,%d^FD%s^FS\n",
			p.Settings.PatientNameX,
			p.Settings.PatientNameY,
			p.Settings.PatientNameFont,
			p.Settings.PatientNameOrient,
			p.Settings.PatientNameHeight,
			p.Settings.PatientNameWidth,
			p.PatientName,
		))
	}
	if strings.TrimSpace(p.TubeCode) != "" {
		b.WriteString(fmt.Sprintf("^FO%d,%d^GB%d,%d,%d,%s,%d^FS\n",
			p.Settings.TubeCodeX,
			p.Settings.TubeCodeY,
			p.Settings.TubeCodeBoxWidth,
			p.Settings.TubeCodeBoxHeight,
			p.Settings.TubeCodeBoxThickness,
			p.Settings.TubeCodeBoxColor,
			p.Settings.TubeCodeBoxRadius,
		))
		b.WriteString(fmt.Sprintf("^FO%d,%d^A%s%s,%d,%d^FD%s^FS\n",
			p.Settings.TubeCodeX+8,
			p.Settings.TubeCodeY+8,
			p.Settings.TubeCodeFont,
			p.Settings.TubeCodeOrientation,
			p.Settings.TubeCodeHeight,
			p.Settings.TubeCodeWidth,
			p.TubeCode,
		))
	}
	b.WriteString("^XZ\n")
	return b.String()
}

func strParam(params map[string]string, requestKey, cfgKey, def string) string {
	if v := strings.TrimSpace(params[requestKey]); v != "" {
		return v
	}
	if v := strings.TrimSpace(params[cfgKey]); v != "" {
		return v
	}
	return def
}

func intParam(params map[string]string, requestKey, cfgKey string, def float64, toDPI bool, dpi int) int {
	raw := strings.TrimSpace(params[requestKey])
	if raw == "" {
		raw = strings.TrimSpace(params[cfgKey])
	}
	if raw == "" {
		raw = strconv.FormatFloat(def, 'f', -1, 64)
	}
	fv, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		fv = def
	}
	if toDPI {
		return mmToDPI(fv, dpi)
	}
	return int(math.Round(fv))
}

func mmToDPI(mm float64, dpi int) int {
	if dpi <= 0 {
		dpi = 200
	}
	return int(math.Round(mm * float64(dpi) / 25.4))
}
