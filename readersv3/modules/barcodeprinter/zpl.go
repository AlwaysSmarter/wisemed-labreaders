package barcodeprinter

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
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
	PrinterResolution      int
	LabelWidth             int
	LabelHeight            int
	Landscape              bool
	StartX                 int
	StartY                 int
	OuterPadding           int
	SectionGap             int
	SectionHeaderHeight    int
	SectionTitleFontH      int
	SectionTitleFontW      int
	BodyFontH              int
	BodyFontW              int
	BodyLineGap            int
	SmallFontH             int
	SmallFontW             int
	StampBoxWidth          int
	StampBoxHeight         int
	StampTitleFontH        int
	StampTitleFontW        int
	StampTitleFontFamily   string
	HeaderFontH            int
	HeaderFontW            int
	HeaderFontFamily       string
	FooterFontH            int
	FooterFontW            int
	FooterFontFamily       string
	AddressFontH           int
	AddressFontW           int
	AddressFontFamily      string
	AddressTitleFontH      int
	AddressTitleFontW      int
	AddressTitleFontFamily string
	LogoX                  int
	LogoY                  int
	LogoWidth              int
	LogoHeight             int
	LogoDataURL            string
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
	// Zebra media width/length must stay aligned with the physical label feed direction.
	// Swapping PW/LL for "landscape" makes some printers continue the job on the next label.
	labelWidth := minInt(width, height)
	labelHeight := maxInt(width, height)
	return PostaRomanaLayoutSettings{
		PrinterResolution:      res,
		LabelWidth:             labelWidth,
		LabelHeight:            labelHeight,
		Landscape:              landscape,
		StartX:                 intParam(params, "pr_start_x_mm", "pr_start_x_mm", 3, true, res),
		StartY:                 intParam(params, "pr_start_y_mm", "pr_start_y_mm", 3, true, res),
		OuterPadding:           intParam(params, "pr_outer_padding_mm", "pr_outer_padding_mm", 4, true, res),
		SectionGap:             intParam(params, "pr_section_gap_mm", "pr_section_gap_mm", 3, true, res),
		SectionHeaderHeight:    intParam(params, "pr_section_header_h_mm", "pr_section_header_h_mm", 8, true, res),
		SectionTitleFontH:      intParam(params, "pr_section_title_font_h", "pr_section_title_font_h", 34, false, 0),
		SectionTitleFontW:      intParam(params, "pr_section_title_font_w", "pr_section_title_font_w", 24, false, 0),
		BodyFontH:              intParam(params, "pr_body_font_h", "pr_body_font_h", 32, false, 0),
		BodyFontW:              intParam(params, "pr_body_font_w", "pr_body_font_w", 24, false, 0),
		BodyLineGap:            intParam(params, "pr_body_line_gap", "pr_body_line_gap", 8, false, 0),
		SmallFontH:             intParam(params, "pr_small_font_h", "pr_small_font_h", 26, false, 0),
		SmallFontW:             intParam(params, "pr_small_font_w", "pr_small_font_w", 20, false, 0),
		StampBoxWidth:          intParam(params, "pr_stamp_box_w_mm", "pr_stamp_box_w_mm", 42, true, res),
		StampBoxHeight:         intParam(params, "pr_stamp_box_h_mm", "pr_stamp_box_h_mm", 28, true, res),
		StampTitleFontH:        intParam(params, "pr_stamp_font_h", "pr_stamp_font_h", 24, false, 0),
		StampTitleFontW:        intParam(params, "pr_stamp_font_w", "pr_stamp_font_w", 18, false, 0),
		StampTitleFontFamily:   strParam(params, "pr_stamp_font_family", "pr_stamp_font_family", "A"),
		HeaderFontH:            intParam(params, "pr_header_font_h", "pr_header_font_h", 34, false, 0),
		HeaderFontW:            intParam(params, "pr_header_font_w", "pr_header_font_w", 24, false, 0),
		HeaderFontFamily:       strParam(params, "pr_header_font_family", "pr_header_font_family", "0"),
		FooterFontH:            intParam(params, "pr_footer_font_h", "pr_footer_font_h", 39, false, 0),
		FooterFontW:            intParam(params, "pr_footer_font_w", "pr_footer_font_w", 30, false, 0),
		FooterFontFamily:       strParam(params, "pr_footer_font_family", "pr_footer_font_family", "0"),
		AddressFontH:           intParam(params, "pr_address_font_h", "pr_address_font_h", 38, false, 0),
		AddressFontW:           intParam(params, "pr_address_font_w", "pr_address_font_w", 28, false, 0),
		AddressFontFamily:      strParam(params, "pr_address_font_family", "pr_address_font_family", "B"),
		AddressTitleFontH:      intParam(params, "pr_address_title_font_h", "pr_address_title_font_h", 38, false, 0),
		AddressTitleFontW:      intParam(params, "pr_address_title_font_w", "pr_address_title_font_w", 28, false, 0),
		AddressTitleFontFamily: strParam(params, "pr_address_title_font_family", "pr_address_title_font_family", "0"),
		LogoX:                  intParam(params, "pr_logo_x_mm", "pr_logo_x_mm", 0, true, res),
		LogoY:                  intParam(params, "pr_logo_y_mm", "pr_logo_y_mm", 0, true, res),
		LogoWidth:              intParam(params, "pr_logo_width_mm", "pr_logo_width_mm", 0, true, res),
		LogoHeight:             intParam(params, "pr_logo_height_mm", "pr_logo_height_mm", 20, true, res),
		LogoDataURL:            strings.TrimSpace(firstNonEmpty(params["pr_logo_data_url"], "")),
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
	canvasW := s.LabelWidth
	canvasH := s.LabelHeight
	if s.Landscape {
		canvasW = s.LabelHeight
		canvasH = s.LabelWidth
	}
	cv := postaCanvas{
		b:          &b,
		physicalW:  s.LabelWidth,
		physicalH:  s.LabelHeight,
		landscape:  s.Landscape,
		fontAdjust: 0,
	}
	if s.Landscape {
		cv.fontAdjust = -4
		renderLandscapePostaRomana(&cv, p, s, canvasW, canvasH)
	} else {
		renderPortraitPostaRomana(&cv, p, s, canvasW, canvasH)
	}

	b.WriteString("^XZ\n")
	return b.String()
}

type postaCanvas struct {
	b          *strings.Builder
	physicalW  int
	physicalH  int
	landscape  bool
	fontAdjust int
}

func renderPortraitPostaRomana(cv *postaCanvas, p *PostaRomanaLabel, s PostaRomanaLayoutSettings, canvasW, canvasH int) {
	x := s.StartX
	y := s.StartY
	contentW := canvasW - (s.StartX * 2)
	contentH := canvasH - (s.StartY * 2)
	bodyX := x + s.OuterPadding
	bodyY := y + s.OuterPadding
	bodyW := contentW - (s.OuterPadding * 2)

	headerH := maxInt(s.SectionHeaderHeight*2, maxInt(s.LogoHeight+(s.OuterPadding*2), effectiveFont(s.HeaderFontH, cv.fontAdjust)+(s.OuterPadding*2)))
	stampW := minInt(s.StampBoxWidth, bodyW/3)
	leftHeaderW := bodyW - stampW - s.SectionGap
	senderBoxY := bodyY + headerH + s.SectionGap
	senderBoxH := maxInt(mmToDPI(35, s.PrinterResolution), (contentH-headerH-(s.SectionGap*3))/3)
	footerBoxH := maxInt(mmToDPI(20, s.PrinterResolution), s.SectionHeaderHeight+(effectiveFont(s.FooterFontH, cv.fontAdjust)*3))
	recipientBoxY := senderBoxY + senderBoxH + s.SectionGap
	recipientBoxH := contentH - headerH - senderBoxH - footerBoxH - (s.SectionGap * 3)
	if recipientBoxH < mmToDPI(45, s.PrinterResolution) {
		recipientBoxH = mmToDPI(45, s.PrinterResolution)
	}
	footerBoxY := recipientBoxY + recipientBoxH + s.SectionGap

	cv.box(x, y, contentW, contentH, 3)
	renderPostaHeader(cv, bodyX, bodyY, leftHeaderW, headerH, p, s)
	renderStampBox(cv, bodyX+leftHeaderW+s.SectionGap, bodyY, stampW, headerH, p.PrepaidStampText, s)

	renderPostaSection(cv, bodyX, senderBoxY, bodyW, senderBoxH, "EXPEDITOR", s, append([]string{p.SenderName}, p.SenderLines...)...)
	renderPostaSection(cv, bodyX, recipientBoxY, bodyW, recipientBoxH, "DESTINATAR", s, append([]string{p.RecipientName}, p.RecipientLines...)...)
	renderPostaFooter(cv, bodyX, footerBoxY, bodyW, footerBoxH, s, p)
}

func renderLandscapePostaRomana(cv *postaCanvas, p *PostaRomanaLabel, s PostaRomanaLayoutSettings, canvasW, canvasH int) {
	x := s.StartX
	y := s.StartY
	contentW := canvasW - (s.StartX * 2)
	contentH := canvasH - (s.StartY * 2)
	bodyX := x + s.OuterPadding
	bodyY := y + s.OuterPadding
	bodyW := contentW - (s.OuterPadding * 2)

	headerH := maxInt(s.SectionHeaderHeight*2, maxInt(s.LogoHeight+(s.OuterPadding*2), effectiveFont(s.HeaderFontH, cv.fontAdjust)+(s.OuterPadding*2)))
	stampW := minInt(mmToDPI(34, s.PrinterResolution), bodyW/4)
	leftHeaderW := bodyW - stampW - s.SectionGap
	footerBoxH := maxInt(mmToDPI(20, s.PrinterResolution), s.SectionHeaderHeight+(effectiveFont(s.FooterFontH, cv.fontAdjust)*3))
	columnY := bodyY + headerH + s.SectionGap
	columnH := contentH - headerH - footerBoxH - (s.SectionGap * 2)
	columnGap := s.SectionGap
	columnW := (bodyW - columnGap) / 2
	footerY := columnY + columnH + s.SectionGap

	cv.box(x, y, contentW, contentH, 3)
	renderPostaHeader(cv, bodyX, bodyY, leftHeaderW, headerH, p, s)
	renderStampBox(cv, bodyX+leftHeaderW+s.SectionGap, bodyY, stampW, headerH, p.PrepaidStampText, s)

	renderPostaSection(cv, bodyX, columnY, columnW, columnH, "EXPEDITOR", s, append([]string{p.SenderName}, p.SenderLines...)...)
	renderPostaSection(cv, bodyX+columnW+columnGap, columnY, columnW, columnH, "DESTINATAR", s, append([]string{p.RecipientName}, p.RecipientLines...)...)
	renderPostaFooter(cv, bodyX, footerY, bodyW, footerBoxH, s, p)
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

func renderPostaSection(cv *postaCanvas, x, y, w, h int, title string, s PostaRomanaLayoutSettings, lines ...string) {
	cv.outlineBox(x, y, w, h, 2)
	cv.writeText(x+s.OuterPadding, y+s.OuterPadding+effectiveFont(s.AddressTitleFontH, cv.fontAdjust), title, s.AddressTitleFontFamily, s.AddressTitleFontH, s.AddressTitleFontW, false)
	textX := x + s.OuterPadding
	textY := y + s.OuterPadding + effectiveFont(s.AddressTitleFontH, cv.fontAdjust) + maxInt(8, s.OuterPadding)
	usableWidth := w - (s.OuterPadding * 2)
	usableHeight := h - effectiveFont(s.AddressTitleFontH, cv.fontAdjust) - (s.OuterPadding * 3)
	cv.writeWrappedParagraph(textX, textY, usableWidth, usableHeight, lines, s.AddressFontFamily, s.AddressFontH, s.AddressFontW, s.BodyLineGap)
}

func renderPostaFooter(cv *postaCanvas, x, y, w, h int, s PostaRomanaLayoutSettings, p *PostaRomanaLabel) {
	cv.outlineBox(x, y, w, h, 2)
	left := []string{
		"Telefon: " + fallbackText(p.RecipientPhone, "-"),
	}
	if strings.TrimSpace(p.ShipmentReference) != "" {
		left = append(left, "Ref: "+p.ShipmentReference)
	}
	right := []string{
		"Cod client: " + fallbackText(p.RecipientCode, "-"),
		"Serviciu: " + fallbackText(p.ServiceName, "POSTA ROMANA"),
	}
	gap := maxInt(s.SectionGap, 8)
	leftW := (w - gap) / 2
	rightW := w - leftW - gap
	cv.writeWrappedParagraph(x+s.OuterPadding, y+s.OuterPadding, leftW-(s.OuterPadding*2), h-(s.OuterPadding*2), left, s.FooterFontFamily, s.FooterFontH, s.FooterFontW, maxInt(4, s.BodyLineGap/2))
	rightX := x + leftW + maxInt(0, gap/2)
	rightY := y + s.OuterPadding
	rightUsableW := rightW - s.OuterPadding
	rightUsableH := h - (s.OuterPadding * 2)
	cv.writeSingleLineFit(rightX, rightY, rightUsableW, rightUsableH, right[0], "B", 50, 36)
}

func renderStampBox(cv *postaCanvas, x, y, w, h int, prepaid string, s PostaRomanaLayoutSettings) {
	cv.outlineBox(x, y, w, h, 2)
	titleH := effectiveFont(s.StampTitleFontH, cv.fontAdjust)
	titleW := effectiveFont(s.StampTitleFontW, cv.fontAdjust)
	smallH := effectiveFont(s.SmallFontH, cv.fontAdjust)
	smallW := effectiveFont(s.SmallFontW, cv.fontAdjust)
	cv.writeCenteredTextWithFont(x+10, y+8, w-20, "TIMBRU / PLATA", s.StampTitleFontFamily, titleH, titleW)
	cv.writeWrappedParagraph(x+8, y+titleH+18, w-16, h-titleH-24, []string{prepaid}, s.StampTitleFontFamily, smallH, smallW, 4)
}

func renderPostaHeader(cv *postaCanvas, x, y, w, h int, p *PostaRomanaLabel, s PostaRomanaLayoutSettings) {
	cv.outlineBox(x, y, w, h, 2)
	logoW := 0
	// Logo coordinates are relative to the header box itself.
	// If the user sets X/Y to 0, the logo should start from the box edge.
	logoX := x + s.LogoX
	logoY := y + s.LogoY
	logoH := 0
	if s.LogoHeight > 0 {
		logoH = minInt(h, s.LogoHeight)
	}
	logoWTarget := s.LogoWidth
	if strings.TrimSpace(s.LogoDataURL) != "" && (logoH > 0 || logoWTarget > 0) {
		if renderedW, ok := cv.writeHeaderLogo(x, y, w, h, logoX, logoY, s.LogoDataURL, logoWTarget, logoH); ok {
			logoW = renderedW
		}
	}
	_ = logoW
}

func (cv *postaCanvas) box(x, y, w, h, thickness int) {
	px, py, pw, ph := cv.rect(x, y, w, h)
	cv.b.WriteString(fmt.Sprintf("^FO%d,%d^GB%d,%d,%d^FS\n", px, py, pw, ph, thickness))
}

func (cv *postaCanvas) outlineBox(x, y, w, h, thickness int) {
	cv.box(x, y, w, h, thickness)
}

func (cv *postaCanvas) filledTitleBar(x, y, w, h int, text string, fontH, fontW int) {
	px, py, pw, ph := cv.rect(x, y, w, h)
	cv.b.WriteString(fmt.Sprintf("^FO%d,%d^GB%d,%d,%d,B,0^FS\n", px, py, pw, ph, 0))
	cv.writeText(x+16, y+maxInt(10, (h-effectiveFont(fontH, cv.fontAdjust))/2), text, "0", fontH, fontW, true)
}

func (cv *postaCanvas) writeCenteredText(x, y, width int, text string, fontH, fontW int) {
	cv.writeCenteredTextWithFont(x, y, width, text, "0", fontH, fontW)
}

func (cv *postaCanvas) writeCenteredTextWithFont(x, y, width int, text, font string, fontH, fontW int) {
	text = zplText(text)
	fontH = effectiveFont(fontH, cv.fontAdjust)
	fontW = effectiveFont(fontW, cv.fontAdjust)
	maxChars := maxInt(8, width/maxInt(12, fontW))
	for _, line := range wrapLabelText(text, maxChars) {
		offset := maxInt(0, (width-(len([]rune(line))*fontW))/2)
		cv.writeText(x+offset, y, line, font, fontH, fontW, false)
		y += fontH + 6
	}
}

func (cv *postaCanvas) writeWrappedParagraph(x, y, width, height int, paragraphs []string, font string, fontH, fontW, lineGap int) {
	fontH = effectiveFont(fontH, cv.fontAdjust)
	fontW = effectiveFont(fontW, cv.fontAdjust)
	lineGap = maxInt(2, lineGap+cv.fontAdjust/2)
	if lineGap < 2 {
		lineGap = 2
	}
	minFontH := maxInt(14, fontH/2)
	minFontW := maxInt(10, fontW/2)
	lines := []string{}
	currentH := fontH
	currentW := fontW
	currentGap := lineGap
	for {
		maxChars := maxInt(10, width/maxInt(10, currentW))
		lines = lines[:0]
		for _, raw := range paragraphs {
			lines = append(lines, wrapLabelText(raw, maxChars)...)
		}
		maxLines := maxInt(1, height/maxInt(8, currentH+currentGap))
		if len(lines) <= maxLines || (currentH <= minFontH && currentW <= minFontW) {
			if len(lines) > maxLines {
				lines = lines[:maxLines]
			}
			fontH = currentH
			fontW = currentW
			lineGap = currentGap
			break
		}
		if currentH > minFontH {
			currentH -= 2
		}
		if currentW > minFontW {
			currentW -= 2
		}
		if currentGap > 2 {
			currentGap -= 1
		}
	}
	for _, line := range lines {
		cv.writeText(x, y+fontH, line, font, fontH, fontW, false)
		y += fontH + lineGap
	}
}

func (cv *postaCanvas) writeSingleLineFit(x, y, width, height int, text, font string, fontH, fontW int) {
	text = zplText(text)
	h := effectiveFont(fontH, cv.fontAdjust)
	w := effectiveFont(fontW, cv.fontAdjust)
	maxW := maxInt(1, width)
	for len([]rune(text))*w > maxW && (h > 18 || w > 12) {
		if h > 18 {
			h -= 2
		}
		if w > 12 {
			w -= 2
		}
	}
	baselineY := y + minInt(height, h+maxInt(4, (height-h)/2))
	cv.writeText(x, baselineY, text, font, h, w, false)
}

func (cv *postaCanvas) writeText(x, y int, text, font string, fontH, fontW int, reverse bool) {
	px, py := cv.point(x, y)
	orientation := "N"
	if cv.landscape {
		orientation = "R"
	}
	reverseFlag := ""
	if reverse {
		reverseFlag = "^FR"
	}
	cv.b.WriteString(fmt.Sprintf("^FO%d,%d%s^A%s%s,%d,%d^FD%s^FS\n", px, py, reverseFlag, font, orientation, effectiveFont(fontH, cv.fontAdjust), effectiveFont(fontW, cv.fontAdjust), zplText(text)))
}

func (cv *postaCanvas) rect(x, y, w, h int) (int, int, int, int) {
	if !cv.landscape {
		return x, y, w, h
	}
	return cv.physicalW - (y + h), x, h, w
}

func (cv *postaCanvas) point(x, y int) (int, int) {
	if !cv.landscape {
		return x, y
	}
	return cv.physicalW - y, x
}

func (cv *postaCanvas) writeHeaderLogo(boxX, boxY, boxW, boxH, logoX, logoY int, dataURL string, targetWidth, targetHeight int) (int, bool) {
	if !cv.landscape {
		return cv.writeLogoDataURL(logoX, logoY, dataURL, targetWidth, targetHeight)
	}
	px, py, _, _ := cv.rect(boxX, boxY, boxW, boxH)
	physicalX := px + maxInt(0, logoY-boxY)
	physicalY := py + maxInt(0, logoX-boxX)
	return cv.writeLogoDataURLPhysical(physicalX, physicalY, dataURL, targetWidth, targetHeight, true)
}

func (cv *postaCanvas) writeLogoDataURL(x, y int, dataURL string, targetWidth, targetHeight int) (int, bool) {
	px, py := cv.point(x, y)
	return cv.writeLogoDataURLPhysical(px, py, dataURL, targetWidth, targetHeight, false)
}

func (cv *postaCanvas) writeLogoDataURLPhysical(px, py int, dataURL string, targetWidth, targetHeight int, rotateCW bool) (int, bool) {
	img, err := decodeDataURLImage(dataURL)
	if err != nil || img == nil {
		return 0, false
	}
	if rotateCW {
		img = rotateImage90CW(img)
	}
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return 0, false
	}
	if targetWidth <= 0 && targetHeight <= 0 {
		return 0, false
	}
	if targetWidth > 0 && targetHeight <= 0 {
		targetHeight = maxInt(1, int(math.Round(float64(targetWidth)*float64(srcH)/float64(srcW))))
	}
	if targetHeight > 0 && targetWidth <= 0 {
		targetWidth = maxInt(1, int(math.Round(float64(targetHeight)*float64(srcW)/float64(srcH))))
	}
	mono := rasterizeMonochrome(img, targetWidth, targetHeight)
	if len(mono) == 0 {
		return 0, false
	}
	bytesPerRow := (targetWidth + 7) / 8
	totalBytes := bytesPerRow * targetHeight
	cv.b.WriteString(fmt.Sprintf("^FO%d,%d^GFA,%d,%d,%d,%s^FS\n", px, py, totalBytes, totalBytes, bytesPerRow, mono))
	return targetWidth, true
}

func decodeDataURLImage(dataURL string) (image.Image, error) {
	raw := strings.TrimSpace(dataURL)
	if raw == "" {
		return nil, nil
	}
	if idx := strings.Index(raw, ","); idx >= 0 && strings.Contains(strings.ToLower(raw[:idx]), "base64") {
		raw = raw[idx+1:]
	}
	blob, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(blob))
	return img, err
}

func rasterizeMonochrome(img image.Image, targetW, targetH int) string {
	if targetW <= 0 || targetH <= 0 {
		return ""
	}
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	bytesPerRow := (targetW + 7) / 8
	data := make([]byte, bytesPerRow*targetH)
	for y := 0; y < targetH; y++ {
		srcY := bounds.Min.Y + int(math.Floor((float64(y)+0.5)*float64(srcH)/float64(targetH)))
		if srcY >= bounds.Max.Y {
			srcY = bounds.Max.Y - 1
		}
		for x := 0; x < targetW; x++ {
			srcX := bounds.Min.X + int(math.Floor((float64(x)+0.5)*float64(srcW)/float64(targetW)))
			if srcX >= bounds.Max.X {
				srcX = bounds.Max.X - 1
			}
			r, g, b, a := img.At(srcX, srcY).RGBA()
			if a == 0 {
				continue
			}
			gray := color.GrayModel.Convert(color.RGBA64{R: uint16(r), G: uint16(g), B: uint16(b), A: uint16(a)}).(color.Gray)
			if gray.Y < 180 {
				idx := y*bytesPerRow + (x / 8)
				data[idx] |= 1 << uint(7-(x%8))
			}
		}
	}
	var out strings.Builder
	for _, b := range data {
		out.WriteString(fmt.Sprintf("%02X", b))
	}
	return out.String()
}

func rotateImage90CW(src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dy(), b.Dx()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(b.Max.Y-y-1, x-b.Min.X, src.At(x, y))
		}
	}
	return dst
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

func effectiveFont(value, adjust int) int {
	return maxInt(10, value+adjust)
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
