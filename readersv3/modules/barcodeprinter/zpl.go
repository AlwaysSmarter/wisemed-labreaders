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

type PostaRomanaLayoutSettings struct {
	PrinterResolution   int
	LabelWidth          int
	LabelHeight         int
	Landscape           bool
	StartX              int
	StartY              int
	OuterPadding        int
	SectionGap          int
	SectionHeaderHeight int
	SectionTitleFontH   int
	SectionTitleFontW   int
	BodyFontH           int
	BodyFontW           int
	BodyLineGap         int
	SmallFontH          int
	SmallFontW          int
	StampBoxWidth       int
	StampBoxHeight      int
	StampTitleFontH     int
	StampTitleFontW     int
}

type PostaRomanaLabel struct {
	Settings          PostaRomanaLayoutSettings
	ServiceName       string
	ShipmentReference string
	PrepaidStampText  string
	SenderName        string
	SenderLines       []string
	RecipientName     string
	RecipientLines    []string
	RecipientPhone    string
	RecipientCode     string
}

func newZPLPrinterFromParams(params map[string]string) (*ZPLPrinter, error) {
	barcodeValue := strings.TrimSpace(firstNonEmpty(params["bc"], params["fileid"], params["code"]))
	if barcodeValue == "" {
		return nil, fmt.Errorf("unknown barcode to print")
	}
	tubeCode := strings.TrimSpace(firstNonEmpty(params["tc"], params["vc"]))
	//if len(tubeCode) > 1 {
	//	tubeCode = tubeCode[:1]
	//}
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
	b.WriteString("^CI28\n")
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

func newPostaRomanaLabelFromParams(params map[string]string) (*PostaRomanaLabel, error) {
	recipientName := strings.TrimSpace(firstNonEmpty(params["recipient_name"], params["dest_name"], params["dn"], params["name"]))
	if recipientName == "" {
		return nil, fmt.Errorf("recipient_name is required")
	}
	address1 := strings.TrimSpace(firstNonEmpty(params["recipient_address1"], params["dest_address1"], params["address"], params["addr1"], params["adresa"]))
	if address1 == "" {
		return nil, fmt.Errorf("recipient_address1 is required")
	}
	recipientCity := strings.TrimSpace(firstNonEmpty(params["recipient_city"], params["dest_city"], params["city"], params["loc"]))
	recipientCounty := strings.TrimSpace(firstNonEmpty(params["recipient_county"], params["dest_county"], params["county"], params["jud"]))
	recipientPostal := strings.TrimSpace(firstNonEmpty(params["recipient_postal_code"], params["dest_postal_code"], params["postal_code"], params["zip"], params["cod"]))
	recipientPhone := strings.TrimSpace(firstNonEmpty(params["recipient_phone"], params["dest_phone"], params["phone"], params["telefon"]))
	recipientCode := strings.TrimSpace(firstNonEmpty(params["recipient_client_code"], params["dest_client_code"], params["client_code"], params["cod_client"]))
	senderName := strings.TrimSpace(firstNonEmpty(params["sender_name"], params["pr_sender_name"], "INSTITUTUL NATIONAL DE SANATATE PUBLICA"))
	senderAddress1 := strings.TrimSpace(firstNonEmpty(params["sender_address1"], params["pr_sender_address1"], "Str. Dr. Leonte Anastasievici, Nr. 1-3"))
	senderAddress2 := strings.TrimSpace(firstNonEmpty(params["sender_address2"], params["pr_sender_address2"], "Cod postal 077042"))
	senderCity := strings.TrimSpace(firstNonEmpty(params["sender_city"], params["pr_sender_city"], "Loc. Bucuresti Sector 5"))
	senderPostal := strings.TrimSpace(firstNonEmpty(params["sender_postal_code"], params["pr_sender_postal_code"]))
	if senderPostal != "" && !strings.Contains(strings.ToLower(senderAddress2), "postal") {
		senderAddress2 = "Cod postal " + senderPostal
	}
	address2 := strings.TrimSpace(firstNonEmpty(params["recipient_address2"], params["dest_address2"], params["addr2"]))
	serviceName := strings.TrimSpace(firstNonEmpty(params["shipping_service_name"], params["service_name"], "POSTA ROMANA"))
	reference := strings.TrimSpace(firstNonEmpty(params["shipment_reference"], params["reference"], params["awb"], recipientCode))
	prepaidStamp := strings.TrimSpace(firstNonEmpty(params["shipping_prepaid_stamp"], params["prepaid_stamp"], "FRANCARE ULTERIOARA"))
	recipientLines := []string{address1}
	if address2 != "" {
		recipientLines = append(recipientLines, address2)
	}
	if recipientCity != "" || recipientCounty != "" {
		recipientLines = append(recipientLines, strings.TrimSpace(firstNonEmpty(
			joinParts(" ", nonEmptyParts("Loc.", recipientCity), nonEmptyParts("Jud.", recipientCounty)),
			joinParts(" ", nonEmptyParts("Loc.", recipientCity)),
			joinParts(" ", nonEmptyParts("Jud.", recipientCounty)),
		)))
	}
	if recipientPostal != "" {
		recipientLines = append(recipientLines, "Cod "+recipientPostal)
	}
	senderLines := []string{senderAddress1}
	if senderAddress2 != "" {
		senderLines = append(senderLines, senderAddress2)
	}
	if senderCity != "" {
		senderLines = append(senderLines, senderCity)
	}
	return &PostaRomanaLabel{
		Settings:          parsePostaRomanaLayoutSettings(params),
		ServiceName:       serviceName,
		ShipmentReference: reference,
		PrepaidStampText:  prepaidStamp,
		SenderName:        senderName,
		SenderLines:       senderLines,
		RecipientName:     recipientName,
		RecipientLines:    recipientLines,
		RecipientPhone:    recipientPhone,
		RecipientCode:     recipientCode,
	}, nil
}

func parsePostaRomanaLayoutSettings(params map[string]string) PostaRomanaLayoutSettings {
	res := intParam(params, "pr_resolution", "pr_resolution", 203, false, 0)
	if res <= 0 {
		res = 203
	}
	width := intParam(params, "pr_label_width_mm", "pr_label_width_mm", 100, true, res)
	height := intParam(params, "pr_label_height_mm", "pr_label_height_mm", 150, true, res)
	landscape := strings.EqualFold(strings.TrimSpace(firstNonEmpty(params["pr_orientation"], "landscape")), "landscape")
	if landscape {
		width, height = height, width
	}
	return PostaRomanaLayoutSettings{
		PrinterResolution:   res,
		LabelWidth:          width,
		LabelHeight:         height,
		Landscape:           landscape,
		StartX:              intParam(params, "pr_start_x_mm", "pr_start_x_mm", 3, true, res),
		StartY:              intParam(params, "pr_start_y_mm", "pr_start_y_mm", 3, true, res),
		OuterPadding:        intParam(params, "pr_outer_padding_mm", "pr_outer_padding_mm", 4, true, res),
		SectionGap:          intParam(params, "pr_section_gap_mm", "pr_section_gap_mm", 3, true, res),
		SectionHeaderHeight: intParam(params, "pr_section_header_h_mm", "pr_section_header_h_mm", 8, true, res),
		SectionTitleFontH:   intParam(params, "pr_section_title_font_h", "pr_section_title_font_h", 34, false, 0),
		SectionTitleFontW:   intParam(params, "pr_section_title_font_w", "pr_section_title_font_w", 24, false, 0),
		BodyFontH:           intParam(params, "pr_body_font_h", "pr_body_font_h", 32, false, 0),
		BodyFontW:           intParam(params, "pr_body_font_w", "pr_body_font_w", 24, false, 0),
		BodyLineGap:         intParam(params, "pr_body_line_gap", "pr_body_line_gap", 8, false, 0),
		SmallFontH:          intParam(params, "pr_small_font_h", "pr_small_font_h", 26, false, 0),
		SmallFontW:          intParam(params, "pr_small_font_w", "pr_small_font_w", 20, false, 0),
		StampBoxWidth:       intParam(params, "pr_stamp_box_w_mm", "pr_stamp_box_w_mm", 42, true, res),
		StampBoxHeight:      intParam(params, "pr_stamp_box_h_mm", "pr_stamp_box_h_mm", 28, true, res),
		StampTitleFontH:     intParam(params, "pr_stamp_font_h", "pr_stamp_font_h", 24, false, 0),
		StampTitleFontW:     intParam(params, "pr_stamp_font_w", "pr_stamp_font_w", 18, false, 0),
	}
}

func (p *PostaRomanaLabel) RenderZPL() string {
	s := p.Settings
	var b strings.Builder
	b.WriteString("^XA\n")
	b.WriteString("^CI28\n")
	b.WriteString(fmt.Sprintf("^PW%d\n", s.LabelWidth))
	b.WriteString(fmt.Sprintf("^LL%d\n", s.LabelHeight))
	b.WriteString("^LH0,0\n")

	x := s.StartX
	y := s.StartY
	contentW := s.LabelWidth - (s.StartX * 2)
	contentH := s.LabelHeight - (s.StartY * 2)
	bodyX := x + s.OuterPadding
	bodyY := y + s.OuterPadding
	bodyW := contentW - (s.OuterPadding * 2)

	headerH := maxInt(s.SectionHeaderHeight*2, s.BodyFontH+s.OuterPadding*2)
	stampW := minInt(s.StampBoxWidth, bodyW/3)
	leftHeaderW := bodyW - stampW - s.SectionGap
	senderBoxY := bodyY + headerH + s.SectionGap
	senderBoxH := (contentH - headerH - (s.SectionGap * 3)) / 3
	recipientBoxY := senderBoxY + senderBoxH + s.SectionGap
	recipientBoxH := contentH - headerH - senderBoxH - (s.SectionGap * 3)

	writeBox(&b, x, y, contentW, contentH, 3)
	writeFilledTitleBar(&b, bodyX, bodyY, leftHeaderW, headerH, "POSTA ROMANA", s.SectionTitleFontH+8, s.SectionTitleFontW+8)
	writeOutlineBox(&b, bodyX+leftHeaderW+s.SectionGap, bodyY, stampW, headerH, 2)
	writeCenteredText(&b, bodyX+leftHeaderW+s.SectionGap+10, bodyY+12, stampW-20, "TIMBRU / PLATA", s.StampTitleFontH, s.StampTitleFontW)
	writeCenteredText(&b, bodyX+leftHeaderW+s.SectionGap+10, bodyY+12+s.StampTitleFontH+10, stampW-20, p.PrepaidStampText, s.SmallFontH, s.SmallFontW)

	writeSection(&b, bodyX, senderBoxY, bodyW, senderBoxH, "EXPEDITOR", s, append([]string{p.SenderName}, p.SenderLines...)...)
	writeSection(&b, bodyX, recipientBoxY, bodyW, recipientBoxH, "DESTINATAR", s, append([]string{p.RecipientName}, p.RecipientLines...)...)

	footerY := recipientBoxY + recipientBoxH - s.SectionHeaderHeight - s.SmallFontH - 18
	if footerY > recipientBoxY+s.SectionHeaderHeight+s.BodyFontH {
		leftFooter := "Telefon: " + fallbackText(p.RecipientPhone, "-")
		rightFooter := "Cod client: " + fallbackText(p.RecipientCode, "-")
		if strings.TrimSpace(p.ShipmentReference) != "" {
			leftFooter = leftFooter + "  Ref: " + p.ShipmentReference
		}
		writeText(&b, bodyX+s.OuterPadding, footerY, leftFooter, "A", s.SmallFontH, s.SmallFontW)
		writeText(&b, bodyX+bodyW/2, footerY, rightFooter, "A", s.SmallFontH, s.SmallFontW)
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

func writeSection(b *strings.Builder, x, y, w, h int, title string, s PostaRomanaLayoutSettings, lines ...string) {
	writeOutlineBox(b, x, y, w, h, 2)
	writeFilledTitleBar(b, x, y, w, s.SectionHeaderHeight, title, s.SectionTitleFontH, s.SectionTitleFontW)
	textX := x + s.OuterPadding
	textY := y + s.SectionHeaderHeight + s.OuterPadding + s.BodyFontH
	usableWidth := w - (s.OuterPadding * 2)
	maxChars := maxInt(12, usableWidth/maxInt(12, s.BodyFontW))
	for _, raw := range lines {
		for _, line := range wrapLabelText(raw, maxChars) {
			writeText(b, textX, textY, line, "A", s.BodyFontH, s.BodyFontW)
			textY += s.BodyFontH + s.BodyLineGap
		}
	}
}

func writeBox(b *strings.Builder, x, y, w, h, thickness int) {
	b.WriteString(fmt.Sprintf("^FO%d,%d^GB%d,%d,%d^FS\n", x, y, w, h, thickness))
}

func writeOutlineBox(b *strings.Builder, x, y, w, h, thickness int) {
	writeBox(b, x, y, w, h, thickness)
}

func writeFilledTitleBar(b *strings.Builder, x, y, w, h int, text string, fontH, fontW int) {
	b.WriteString(fmt.Sprintf("^FO%d,%d^GB%d,%d,%d,B,0^FS\n", x, y, w, h, 0))
	b.WriteString(fmt.Sprintf("^FO%d,%d^FR^A0N,%d,%d^FD%s^FS\n", x+16, y+maxInt(10, (h-fontH)/2), fontH, fontW, zplText(text)))
}

func writeCenteredText(b *strings.Builder, x, y, width int, text string, fontH, fontW int) {
	text = zplText(text)
	maxChars := maxInt(8, width/maxInt(12, fontW))
	for _, line := range wrapLabelText(text, maxChars) {
		offset := maxInt(0, (width-(len([]rune(line))*fontW))/2)
		b.WriteString(fmt.Sprintf("^FO%d,%d^A0N,%d,%d^FD%s^FS\n", x+offset, y, fontH, fontW, zplText(line)))
		y += fontH + 6
	}
}

func writeText(b *strings.Builder, x, y int, text, font string, fontH, fontW int) {
	b.WriteString(fmt.Sprintf("^FO%d,%d^A%sN,%d,%d^FD%s^FS\n", x, y, font, fontH, fontW, zplText(text)))
}

func wrapLabelText(text string, maxChars int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxChars <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}
	lines := []string{}
	current := words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if len([]rune(candidate)) <= maxChars {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = word
	}
	lines = append(lines, current)
	return lines
}

func zplText(s string) string {
	replacer := strings.NewReplacer("^", "-", "~", "-", "\r", " ", "\n", " ")
	return replacer.Replace(strings.TrimSpace(s))
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func joinParts(sep string, parts ...string) string {
	items := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return strings.Join(items, sep)
}

func nonEmptyParts(prefix, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.TrimSpace(prefix) == "" {
		return value
	}
	return strings.TrimSpace(prefix) + " " + value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
