const translations = {
  ro: {
    language: "Limba",
    localAdmin: "ADMIN LOCAL",
    readerEyebrow: "READER",
    localHttp: "HTTP LOCAL",
    loginTitle: "WiseMED Reader Console",
    loginSubtitle: "Login cu utilizator WiseMED pentru administrarea locala a readerului.",
    username: "Utilizator",
    password: "Parola",
    login: "Login",
    logout: "Logout",
    navOverview: "Acasa",
    navDailyDetails: "Detalii zilnice",
    navAnalytes: "Analize",
    navSettings: "Setari",
    navOrders: "Cereri analize",
    navQc: "Controlul calitatii",
    navHelp: "Ajutor",
    settingsAnalytes: "Analize",
    settingsReader: "Setari reader",
    settingsDailyDetails: "Detalii zilnice",
    settingsQc: "Setari QC",
    repeatModeLabel: "Mod tratare repetari",
    repeatModeHelp: "Stabileste daca selectarea unui alt rezultat schimba doar analiza curenta sau tot grupul de analize lucrate simultan pentru aceeasi proba.",
    repeatModeIndividual: "Repetari individuale pe analiza",
    repeatModeGrouped: "Repetari grupate pe lot de rezultate",
    saveReaderSettings: "Salveaza setarile readerului",
    readerSettingsSaved: "Setarile readerului au fost salvate.",
    search: "Cautare",
    variableName: "Nume variabila",
    value: "Valoare",
    source: "Sursa",
    scope: "Scop",
    noDailyDetails: "Nu exista detalii zilnice pentru filtrul selectat.",
    dailyDetailsDay: "ZI",
    dailyDetailsDayRound: "ZI + RUNDA",
    dailyDetailsDayAnalyte: "ZI + ANALIZA",
    dailyDetailsDayRoundAnalyte: "ZI + RUNDA + ANALIZA",
    dailyDetailsDayTab: "Pe zi",
    dailyDetailsDayRoundTab: "Pe zi si runda",
    dailyDetailsDayAnalyteTab: "Pe zi si analiza",
    dailyDetailsDayRoundAnalyteTab: "Pe zi, runda si analiza",
    sessionLabel: "Autentificat ca",
    readerSummary: "Rezumat reader",
    readerCategory: "Categorie",
    medicalUnitLabel: "Unitate medicala",
    equipmentTypeLabel: "Tip echipament",
    equipmentIdLabel: "ID echipament",
    readerIdLabel: "ID reader",
    analyzerNameLabel: "Analizor",
    readerCategoryGeneric: "Generic",
    readerCategoryUrine: "Urina",
    readerCategoryHematology: "Hematologie",
    readerCategoryBiochemistry: "Biochimie",
    readerCategoryImmunology: "Imunologie",
    readerCategoryGas: "Gaz cromatograf",
    readerCategorySpectro: "Spectrofotometru",
    dashboardTitle: "Acasa",
    wisemedws: "WiseMEDWS",
    analyzer: "Analizor",
    status: "Status",
    recentLogs: "Loguri recente",
    logPolling: "Polling (s)",
    refresh: "Refresh",
    analytes: "Analize",
    searchAnalytes: "Cauta dupa tag, code, nume",
    analyteEditor: "Editor analiza",
    new: "Nou",
    tag: "Tag",
    code: "Code",
    name: "Nume",
    description: "Descriere",
    resultType: "Tip rezultat",
    formatting: "Formatare",
    weighting: "Pondere",
    measureUnit: "Unitate masura",
    reagentsSet: "Set reactivi",
    active: "Activ",
    save: "Salveaza",
    delete: "Sterge",
    howTo: "Ghid",
    openSeparate: "Deschide separat",
    orderDetails: "Detalii cerere",
    round: "Runda",
    slot: "Pozitie",
    sample: "Proba",
    analyses: "Analize",
    otherResults: "Alte rezultate",
    currentResult: "Rezultat curent",
    selectResult: "Selecteaza rezultat",
    analysesForSample: "Analize pentru proba",
    analysisName: "Denumire analiza",
    analysisTag: "Tag",
    analysisQualitative: "Rezultat calitativ",
    analysisQuantitative: "Rezultat cantitativ",
    analysesCount: "Nr. analize",
    orderDate: "Data",
    selectAll: "Selecteaza tot",
    importFile: "Import",
    exportFile: "Export",
    getWorklist: "Genereaza worklist",
    newRound: "Runda noua",
    exportedFile: "Fisier exportat",
    importedFile: "Fisier importat",
    importing: "Se importa...",
    importSuccess: "Import finalizat",
    importFailed: "Import esuat",
    rowsImported: "randuri importate",
    warningCount: "avertismente",
    worklistPending: "Generarea worklist-ului necesita endpoint-ul WiseMED API dedicat.",
    close: "Inchide",
    medicalUnit: "Unitate medicala",
    comm: "Comunicatie",
    layout: "Layout",
    orders: "Cereri analize",
    results: "Rezultate",
    events: "Evenimente",
    todayResults: "Astazi analize",
    dailyTrend: "Evolutie pe zile",
    qcToday: "QC astazi",
    qcTargetTitle: "Setari QC pentru analiza",
    qcTargetsExisting: "Setari QC existente",
    qcTargetAnalyte: "Analiza selectata",
    qcTargetLevel: "Nivel QC",
    qcTargetLot: "Lot",
    qcTargetMean: "Medie",
    qcTargetSD: "SD",
    qcTargetOneSD: "1 SD",
    qcTargetTwoSD: "2 SD",
    qcTargetThreeSD: "3 SD",
    qcTargetCV: "CV %",
    saveQcTarget: "Salveaza QC",
    deleteQcTarget: "Sterge QC",
    westgardTitle: "Diagrama Westgard",
    westgardAnalyte: "Analiza",
    westgardLevel: "Nivel QC",
    westgardLot: "Lot",
    westgardDateFrom: "De la",
    westgardDateTo: "Pana la",
    editAnalyte: "Editeaza",
    noWestgard: "Nu exista suficiente date numerice pentru filtru.",
    qcRecords: "Controale QC",
    qcResults: "Rezultate QC",
    qcTargets: "Tinte QC",
    qcDate: "Data QC",
    qcAddManual: "Adauga QC",
    qcManualTitle: "Adaugare QC manuala",
    qcControlLabel: "Eticheta control",
    qcRound: "Runda QC",
    qcControlType: "Nivel QC",
    allControlTypes: "Toate",
    allAnalytes: "Toate analizele",
    qcDetails: "Detalii QC",
    qcLevel: "Nivel QC",
    qcReadAt: "Citit la",
    diluentInfo: "Diluent",
    lotNo: "Lot",
    numericResults: "Rezultate numerice",
    interpretation: "Interpretare",
    outside2sd: "Peste 2SD",
    outside3sd: "Peste 3SD",
    westgardPeriod: "Perioada",
    westgardCurrentWeek: "Saptamana curenta",
    westgardPreviousWeek: "Saptamana anterioara",
    westgardCurrentMonth: "Luna curenta",
    westgardPreviousMonth: "Luna anterioara",
    westgardCurrentYear: "Anul curent",
    westgardCustom: "Definit utilizator",
    westgardInvalid: "Invalid",
    westgardValid: "Valid",
    westgardStatsOwnMean: "Media citirilor",
    westgardMedian: "Mediana",
    westgardOutliers: "Aberatii",
    westgardRepeatability: "Repetabilitate",
    noQc: "Nu exista inregistrari QC.",
    connected: "conectat",
    disconnected: "deconectat",
    noLogs: "Nu exista loguri.",
    noAnalytes: "Nu exista analize.",
    noOrders: "Nu exista probe pentru perioada selectata",
    editing: "Editezi",
    readyNew: "Pregatit pentru o analiza noua",
    saved: "Salvat.",
    deleteConfirm: "Sterg analyte-ul",
    noResult: "Fara rezultat",
    analysesWithoutResult: "Analize fara rezultat",
    analysesWithResult: "Analize cu rezultat",
    sampleIdPrompt: "Sample ID",
    sampleNo: "Nr. proba",
    statusReceived: "primit",
    statusPending: "in asteptare",
    statusCompleted: "finalizat",
    statusFailed: "esuat",
    statusProcessing: "in procesare",
    statusImported: "importat",
    fieldTag: "Tag",
    fieldCode: "Cod",
    fieldName: "Nume",
    fieldDescription: "Descriere",
    fieldResultType: "Tip rezultat",
    fieldFormatting: "Formatare",
    fieldWeighting: "Pondere",
    fieldMeasureUnit: "Unitate masura",
    fieldReagentsSet: "Set reactivi",
    fieldActive: "Activ",
  },
  en: {
    language: "Language",
    localAdmin: "LOCAL ADMIN",
    readerEyebrow: "READER",
    localHttp: "LOCAL HTTP",
    loginTitle: "WiseMED Reader Console",
    loginSubtitle: "Sign in with your WiseMED account to manage the local reader.",
    username: "Username",
    password: "Password",
    login: "Login",
    logout: "Logout",
    navOverview: "Home",
    navDailyDetails: "Daily Details",
    navAnalytes: "Analytes",
    navSettings: "Settings",
    navOrders: "Analysis Requests",
    navQc: "Quality Control",
    navHelp: "Help",
    settingsAnalytes: "Analytes",
    settingsReader: "Reader settings",
    settingsDailyDetails: "Daily Details",
    settingsQc: "QC Settings",
    repeatModeLabel: "Repeat handling mode",
    repeatModeHelp: "Choose whether selecting another result changes only the current analysis or the whole batch of analyses produced at the same time for the same sample.",
    repeatModeIndividual: "Individual repeats per analysis",
    repeatModeGrouped: "Grouped repeats per result batch",
    saveReaderSettings: "Save reader settings",
    readerSettingsSaved: "Reader settings were saved.",
    search: "Search",
    variableName: "Variable name",
    value: "Value",
    source: "Source",
    scope: "Scope",
    noDailyDetails: "No daily details available for the selected filter.",
    dailyDetailsDay: "DAY",
    dailyDetailsDayRound: "DAY + ROUND",
    dailyDetailsDayAnalyte: "DAY + ANALYTE",
    dailyDetailsDayRoundAnalyte: "DAY + ROUND + ANALYTE",
    dailyDetailsDayTab: "By day",
    dailyDetailsDayRoundTab: "By day and round",
    dailyDetailsDayAnalyteTab: "By day and analyte",
    dailyDetailsDayRoundAnalyteTab: "By day, round and analyte",
    sessionLabel: "Signed in as",
    readerSummary: "Reader summary",
    readerCategory: "Category",
    medicalUnitLabel: "Medical unit",
    equipmentTypeLabel: "Equipment type",
    equipmentIdLabel: "Equipment ID",
    readerIdLabel: "Reader ID",
    analyzerNameLabel: "Analyzer",
    readerCategoryGeneric: "Generic",
    readerCategoryUrine: "Urine",
    readerCategoryHematology: "Hematology",
    readerCategoryBiochemistry: "Biochemistry",
    readerCategoryImmunology: "Immunology",
    readerCategoryGas: "Gas chromatograph",
    readerCategorySpectro: "Spectrophotometer",
    dashboardTitle: "Home",
    wisemedws: "WiseMEDWS",
    analyzer: "Analyzer",
    status: "Status",
    recentLogs: "Recent Logs",
    logPolling: "Polling (s)",
    refresh: "Refresh",
    analytes: "Analytes",
    searchAnalytes: "Search by tag, code, name",
    analyteEditor: "Analyte editor",
    new: "New",
    tag: "Tag",
    code: "Code",
    name: "Name",
    description: "Description",
    resultType: "Result Type",
    formatting: "Formatting",
    weighting: "Weighting",
    measureUnit: "Measure Unit",
    reagentsSet: "Reagents Set",
    active: "Active",
    save: "Save",
    delete: "Delete",
    howTo: "How To",
    openSeparate: "Open separately",
    orderDetails: "Request details",
    round: "Round",
    slot: "Slot",
    sample: "Sample",
    analyses: "Analyses",
    otherResults: "Other results",
    currentResult: "Current result",
    selectResult: "Select result",
    analysesForSample: "Analyses for sample",
    analysisName: "Analysis name",
    analysisTag: "Tag",
    analysisQualitative: "Qualitative result",
    analysisQuantitative: "Quantitative result",
    analysesCount: "Analyses count",
    orderDate: "Date",
    selectAll: "Select all",
    importFile: "Import",
    exportFile: "Export",
    getWorklist: "Get worklist",
    newRound: "New round",
    exportedFile: "Exported file",
    importedFile: "Imported file",
    importing: "Importing...",
    importSuccess: "Import completed",
    importFailed: "Import failed",
    rowsImported: "rows imported",
    warningCount: "warnings",
    worklistPending: "Get worklist needs the dedicated WiseMED API endpoint.",
    close: "Close",
    medicalUnit: "Medical Unit",
    comm: "Communication",
    layout: "Layout",
    orders: "Analysis Requests",
    results: "Results",
    events: "Events",
    todayResults: "Today's analyses",
    dailyTrend: "Daily trend",
    qcToday: "QC today",
    qcTargetTitle: "QC settings for analyte",
    qcTargetsExisting: "Saved QC settings",
    qcTargetAnalyte: "Selected analyte",
    qcTargetLevel: "QC level",
    qcTargetLot: "Lot",
    qcTargetMean: "Mean",
    qcTargetSD: "SD",
    qcTargetOneSD: "1 SD",
    qcTargetTwoSD: "2 SD",
    qcTargetThreeSD: "3 SD",
    qcTargetCV: "CV %",
    saveQcTarget: "Save QC",
    deleteQcTarget: "Delete QC",
    westgardTitle: "Westgard chart",
    westgardAnalyte: "Analyte",
    westgardLevel: "QC level",
    westgardLot: "Lot",
    westgardDateFrom: "From",
    westgardDateTo: "To",
    editAnalyte: "Edit",
    noWestgard: "Not enough numeric data for this filter.",
    qcRecords: "QC records",
    qcResults: "QC results",
    qcTargets: "QC targets",
    qcDate: "QC date",
    qcAddManual: "Add QC",
    qcManualTitle: "Manual QC entry",
    qcControlLabel: "Control label",
    qcRound: "QC round",
    qcControlType: "QC level",
    allControlTypes: "All",
    allAnalytes: "All analytes",
    qcDetails: "QC details",
    qcLevel: "QC level",
    qcReadAt: "Read at",
    diluentInfo: "Diluent",
    lotNo: "Lot",
    numericResults: "Numeric results",
    interpretation: "Interpretation",
    outside2sd: "Above 2SD",
    outside3sd: "Above 3SD",
    westgardPeriod: "Period",
    westgardCurrentWeek: "Current week",
    westgardPreviousWeek: "Previous week",
    westgardCurrentMonth: "Current month",
    westgardPreviousMonth: "Previous month",
    westgardCurrentYear: "Current year",
    westgardCustom: "Custom",
    westgardInvalid: "Invalid",
    westgardValid: "Valid",
    westgardStatsOwnMean: "Own mean",
    westgardMedian: "Median",
    westgardOutliers: "Outliers",
    westgardRepeatability: "Repeatability",
    noQc: "No QC records available.",
    connected: "connected",
    disconnected: "disconnected",
    noLogs: "No logs available.",
    noAnalytes: "No analytes available.",
    noOrders: "No orders for the selected period.",
    editing: "Editing",
    readyNew: "Ready for a new analyte",
    saved: "Saved.",
    deleteConfirm: "Delete analyte",
    noResult: "No result",
    analysesWithoutResult: "Analyses without result",
    analysesWithResult: "Analyses with result",
    sampleIdPrompt: "Sample ID",
    sampleNo: "Sample No",
    statusReceived: "received",
    statusPending: "pending",
    statusCompleted: "completed",
    statusFailed: "failed",
    statusProcessing: "processing",
    statusImported: "imported",
    fieldTag: "Tag",
    fieldCode: "Code",
    fieldName: "Name",
    fieldDescription: "Description",
    fieldResultType: "Result Type",
    fieldFormatting: "Formatting",
    fieldWeighting: "Weighting",
    fieldMeasureUnit: "Measure Unit",
    fieldReagentsSet: "Reagents Set",
    fieldActive: "Active",
  },
};

function localISODate() {
  const now = new Date();
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  const day = String(now.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function offsetISODate(days) {
  const now = new Date();
  now.setDate(now.getDate() + days);
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  const day = String(now.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function isoDateFromDate(date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function currentPeriodBounds(period) {
  const now = new Date();
  const start = new Date(now);
  const end = new Date(now);
  if (period === "current_week") {
    const delta = (now.getDay() + 6) % 7;
    start.setDate(now.getDate() - delta);
  } else if (period === "previous_week") {
    const delta = (now.getDay() + 6) % 7;
    start.setDate(now.getDate() - delta - 7);
    end.setDate(now.getDate() - delta - 1);
  } else if (period === "current_month") {
    start.setDate(1);
  } else if (period === "previous_month") {
    start.setMonth(now.getMonth() - 1, 1);
    end.setDate(0);
  } else if (period === "current_year") {
    start.setMonth(0, 1);
  }
  if (period !== "previous_week" && period !== "previous_month") {
    end.setHours(0, 0, 0, 0);
  }
  start.setHours(0, 0, 0, 0);
  return { from: isoDateFromDate(start), to: isoDateFromDate(end) };
}

const BARCODE_TYPE_OPTIONS = [
  ["B3", "Code 39"],
  ["B0", "Aztec Barcode"],
  ["B1", "Code 11"],
  ["B2", "Interleaved 2 of 5"],
  ["B4", "Code 49"],
  ["B5", "Planet Code"],
  ["B7", "PDF417"],
  ["B8", "EAN-8"],
  ["B9", "UPC-E8"],
  ["BA", "Code 93"],
  ["BB", "CODABLOCK"],
  ["BC", "Code 128"],
  ["BD", "UPS MaxiCode"],
  ["BE", "EAN-13"],
  ["BF", "MicroPDF417"],
  ["BI", "Industrial 2 of 5"],
  ["BJ", "Standard 2 of 5"],
  ["BK", "ANSI Codabar"],
  ["BL", "LOGMARS"],
  ["BM", "MSI"],
  ["BO", "Aztec 2"],
  ["BP", "Plessey"],
  ["BQ", "QR Code"],
  ["BR", "GS1 Databar"],
  ["BS", "UPC/EAN Extensions"],
  ["BT", "TLC39"],
  ["BU", "UPC-A"],
  ["BX", "Data Matrix"],
  ["BZ", "Postal Code"],
];

const BARCODE_YES_NO = [["N", "No"], ["Y", "Yes"]];
const BARCODE_ORIENTATION = [["N", "Normal"], ["R", "Rotated"], ["I", "Inverted"], ["B", "Bottom Up"]];
const BARCODE_RESOLUTION = [["200", "8mm - 200dpi"], ["300", "12mm - 300dpi"], ["600", "24mm - 600dpi"]];
const BARCODE_MODULE_WIDTH = [["1", "1 dot"], ["2", "2 dots"], ["3", "3 dots"], ["4", "4 dots"], ["5", "5 dots"], ["6", "6 dots"], ["7", "7 dots"], ["8", "8 dots"], ["9", "9 dots"], ["10", "10 dots"]];
const BARCODE_RATIO = [["2.0", "2.0"], ["2.1", "2.1"], ["2.2", "2.2"], ["2.3", "2.3"], ["2.4", "2.4"], ["2.5", "2.5"], ["2.6", "2.6"], ["2.7", "2.7"], ["2.8", "2.8"], ["2.9", "2.9"], ["3.0", "3.0"]];
const BARCODE_FONT_OPTIONS = [["A", "A"], ["B", "B"], ["C", "C"], ["D", "D"], ["E", "E"], ["F", "F"], ["G", "G"], ["H", "H"], ["0", "0"]];
const BARCODE_BOX_COLOR = [["B", "Black"], ["W", "White"]];
const BARCODE_QR_EC = [["Q", "High reliability"], ["H", "Ultra-high reliability"], ["M", "Standard"], ["L", "High density"]];

const BARCODE_FIELD_SECTIONS = [
  {
    title: "Printer settings",
    fields: [
      { key: "bcp_type", label: "Printer type", type: "select", options: [["zebrazpl", "Zebra ZPL"]], default: "zebrazpl" },
      { key: "othercfg_sel_printer", label: "Selected printer", type: "printer-select", default: "" },
      { key: "othercfg_printer_resolution", label: "Printer resolution", type: "select", options: BARCODE_RESOLUTION, default: "200" },
    ],
  },
  {
    title: "Barcode settings",
    fields: [
      { key: "othercfg_print_bcodex", label: "Barcode X (mm)", type: "number", default: "2", span: 1 },
      { key: "othercfg_print_bcodey", label: "Barcode Y (mm)", type: "number", default: "3", span: 1 },
      { key: "othercfg_printer_barcode", label: "Barcode type", type: "select", options: BARCODE_TYPE_OPTIONS, default: "B3", span: 1 },
      { key: "othercfg_print_bcodeopt_w", label: "Module width", type: "select", options: BARCODE_MODULE_WIDTH, default: "2", span: 1 },
      { key: "othercfg_bc_widenarowr", label: "Wide bar to narrow bar ratio", type: "select", options: BARCODE_RATIO, default: "3.0", span: 1 },
      { key: "othercfg_print_bcodeopt_o", label: "Orientation", type: "select", options: BARCODE_ORIENTATION, default: "N", span: 1, showFor: ["B0","B1","B2","B3","B4","B5","B7","B8","B9","BA","BB","BC","BE","BF","BI","BJ","BK","BL","BM","BO","BP","BQ","BR","BS","BT","BU","BX","BZ"] },
      { key: "othercfg_print_bcodeopt_e", label: "Check digit", type: "select", options: BARCODE_YES_NO, default: "N", span: 1, showFor: ["B1","B2","B3","B9","BA","BC","BK","BM","BP","BU"] },
      { key: "othercfg_print_bcodeopt_f", label: "Interpretation line", type: "select", options: BARCODE_YES_NO, default: "N", span: 1, showFor: ["B1","B2","B3","B4","B5","B8","B9","BA","BC","BE","BI","BJ","BK","BM","BP","BS","BU","BZ"] },
      { key: "othercfg_print_bcodeopt_g", label: "Interpretation line above", type: "select", options: BARCODE_YES_NO, default: "N", span: 1, showFor: ["B1","B2","B3","B5","B8","B9","BA","BC","BE","BI","BJ","BK","BL","BM","BP","BS","BU","BZ"] },
      { key: "othercfg_print_bcodeopt_gwc", label: "Interpretation line with check", type: "select", options: BARCODE_YES_NO, default: "N", span: 1, showFor: ["BM"] },
      { key: "othercfg_print_bcodeopt_h", label: "Barcode height (dots)", type: "number", default: "100", span: 1, showFor: ["B1","B2","B3","B4","B5","B7","B8","B9","BA","BB","BC","BE","BF","BI","BJ","BK","BL","BM","BP","BR","BS","BT","BU","BX","BZ"] },
      { key: "othercfg_print_bcodeopt_b", label: "Magnification factor", type: "select", options: BARCODE_YES_NO, default: "N", span: 1, showFor: ["B0"] },
      { key: "othercfg_print_bcodeopt_d", label: "Error control", type: "number", default: "0", span: 1, showFor: ["B0"] },
      { key: "othercfg_print_bcodeopt_ec", label: "QR error correction", type: "select", options: BARCODE_QR_EC, default: "Q", span: 1, showFor: ["BQ"] },
      { key: "othercfg_print_bcodeopt_m", label: "Menu symbol", type: "select", options: BARCODE_YES_NO, default: "N", span: 1, showFor: ["B0"] },
      { key: "othercfg_print_bcodeopt_c", label: "Interpretation code", type: "select", options: BARCODE_YES_NO, default: "N", span: 1, showFor: ["B0","B2"] },
      { key: "othercfg_print_bcodeopt_syn", label: "Symbol number", type: "number", default: "2", span: 1, showFor: ["BD"] },
      { key: "othercfg_print_bcodeopt_n", label: "No. symbols", type: "number", default: "1", span: 1, showFor: ["B0","BD"] },
      { key: "othercfg_print_bcodeopt_s", label: "ID for structural append", type: "number", default: "0", span: 1, showFor: ["B0"] },
      { key: "othercfg_print_bcodeopt_sm", label: "Starting mode", type: "select", options: [["A", "Automatic mode"], ["0", "Regular alphanumeric"], ["1", "Multiple read alphanumeric"], ["2", "Regular numeric"], ["3", "Group alphanumeric"], ["4", "Regular alphanumeric S1"], ["5", "Regular alphanumeric S2"]], default: "A", span: 1, showFor: ["B4"] },
      { key: "othercfg_print_bcodeopt_sl", label: "Security level", type: "number", default: "0", span: 1, showFor: ["B7","BB"] },
      { key: "othercfg_print_bcodeopt_ce", label: "No. cols. to encode", type: "number", default: "0", span: 1, showFor: ["B7","BX"] },
      { key: "othercfg_print_bcodeopt_re", label: "No. rows to encode", type: "number", default: "0", span: 1, showFor: ["B7","BB","BX","BZ"] },
      { key: "othercfg_print_bcodeopt_tr", label: "Truncate rows indicator", type: "select", options: BARCODE_YES_NO, default: "N", span: 1, showFor: ["B7"] },
      { key: "othercfg_print_bcodeopt_rc", label: "Chars per row", type: "number", default: "0", span: 1, showFor: ["BB"] },
      { key: "othercfg_print_bcodeopt_cbm", label: "CODABLOCK mode", type: "select", options: [["F", "F"], ["A", "A"], ["E", "E"]], default: "F", span: 1, showFor: ["BB"] },
      { key: "othercfg_print_bcodeopt_cdm", label: "Code128 mode", type: "select", options: [["N", "No selection mode"], ["U", "UCC Case"], ["A", "Automatic"], ["D", "UCC/EAN"]], default: "N", span: 1, showFor: ["BC"] },
      { key: "othercfg_print_bcodeopt_upsm", label: "UPS mode", type: "select", options: [["2", "Numeric"], ["3", "Alphanumeric"], ["4", "Symbol"], ["5", "Full ECC"], ["6", "Reader"]], default: "2", span: 1, showFor: ["BD"] },
      { key: "othercfg_print_bcodeopt_qm", label: "QR code model", type: "select", options: [["1", "Model 1"], ["2", "Model 2"]], default: "2", span: 1, showFor: ["BQ"] },
      { key: "othercfg_print_bcodeopt_nmod", label: "Numeric mode", type: "number", default: "0", span: 1, showFor: ["BF"] },
      { key: "othercfg_print_bcodeopt_ssc", label: "Start char", type: "text", default: "A", span: 1, showFor: ["BK"] },
      { key: "othercfg_print_bcodeopt_stc", label: "Stop char", type: "text", default: "B", span: 1, showFor: ["BK"] },
      { key: "othercfg_print_bcodeopt_mv", label: "Mask value", type: "number", default: "0", span: 1, showFor: ["BQ"] },
      { key: "othercfg_print_bcodeopt_sh", label: "Separator height", type: "number", default: "1", span: 1, showFor: ["BR"] },
      { key: "othercfg_print_bcodeopt_sw", label: "Segment width", type: "number", default: "2", span: 1, showFor: ["BR"] },
      { key: "othercfg_print_bcodeopt_sym", label: "Symbology GS1 type", type: "select", options: [["1", "GS1 DataBar Omnidirectional"], ["2", "GS1 DataBar Truncated"], ["3", "GS1 DataBar Stacked"], ["4", "GS1 DataBar Stacked Omnidirectional"], ["5", "GS1 DataBar Limited"], ["6", "GS1 DataBar Expanded"], ["7", "UPC-A"], ["8", "UPC-E"], ["9", "EAN-13"], ["10", "EAN-8"], ["11", "UCC/EAN-128"]], default: "1", span: 1, showFor: ["BR"] },
      { key: "othercfg_print_bcodeopt_ctnwn", label: "Code39 wide to narrow", type: "number", default: "3", span: 1, showFor: ["BT"] },
      { key: "othercfg_print_bcodeopt_rh", label: "Row height", type: "number", default: "10", span: 1, showFor: ["BT"] },
      { key: "othercfg_print_bcodeopt_nbw", label: "Narrow bar width", type: "number", default: "2", span: 1, showFor: ["BT"] },
      { key: "othercfg_print_bcodeopt_dmq", label: "DataMatrix quality", type: "select", options: [["0", "Auto"], ["50", "50"], ["80", "80"], ["100", "100"], ["140", "140"], ["200", "200"]], default: "0", span: 1, showFor: ["BX"] },
      { key: "othercfg_print_bcodeopt_dmf", label: "DataMatrix format", type: "select", options: [["6", "256 ISO 8bit"], ["1", "Datanumeric"], ["2", "Uppercase alphanumeric no sym"], ["3", "Uppercase alphanumeric with sym"], ["4", "Uppercase alphanumeric no sym 2"], ["5", "128 ASCII 7bit"]], default: "6", span: 1, showFor: ["BX"] },
      { key: "othercfg_print_bcodeopt_dma", label: "DataMatrix aspect ratio", type: "select", options: [["1", "Square"], ["2", "Rectangular"]], default: "1", span: 1, showFor: ["BX"] },
      { key: "othercfg_print_bcodeopt_dme", label: "DataMatrix escape sequence", type: "text", default: "", span: 1, showFor: ["BX"] },
      { key: "othercfg_print_bcodeopt_pct", label: "Postal code type", type: "select", options: [["0", "Postnet bar code"], ["1", "Planet bar code"], ["2", "Reserved"], ["3", "USPS Intelligent Mail barcode"]], default: "0", span: 1, showFor: ["BZ"] },
    ],
  },
  {
    title: "Label text settings",
    fields: [
      { key: "othercfg_label_width", label: "Label width (mm)", type: "number", default: "35" },
      { key: "othercfg_label_height", label: "Label height (mm)", type: "number", default: "25" },
      { key: "othercfg_label_ox", label: "Label origin X (mm)", type: "number", default: "0" },
      { key: "othercfg_label_oy", label: "Label origin Y (mm)", type: "number", default: "0" },
      { key: "othercfg_print_namex", label: "Patient name X (mm)", type: "number", default: "2" },
      { key: "othercfg_print_namey", label: "Patient name Y (mm)", type: "number", default: "18" },
      { key: "othercfg_print_namef", label: "Patient name font", type: "select", options: BARCODE_FONT_OPTIONS, default: "A" },
      { key: "othercfg_print_nameo", label: "Patient name orientation", type: "select", options: BARCODE_ORIENTATION, default: "N" },
      { key: "othercfg_print_nameh", label: "Patient name height (dots)", type: "number", default: "18" },
      { key: "othercfg_print_namew", label: "Patient name width (dots)", type: "number", default: "12" },
      { key: "othercfg_print_bcodetxtx", label: "Barcode text X (mm)", type: "number", default: "2" },
      { key: "othercfg_print_bcodetxty", label: "Barcode text Y (mm)", type: "number", default: "22" },
      { key: "othercfg_print_bcodetxtf", label: "Barcode text font", type: "select", options: BARCODE_FONT_OPTIONS, default: "D" },
      { key: "othercfg_print_bcodetxto", label: "Barcode text orientation", type: "select", options: BARCODE_ORIENTATION, default: "N" },
      { key: "othercfg_print_bcodetxth", label: "Barcode text height (dots)", type: "number", default: "18" },
      { key: "othercfg_print_bcodetxtw", label: "Barcode text width (dots)", type: "number", default: "12" },
      { key: "othercfg_print_tubecodex", label: "Tube code X (mm)", type: "number", default: "28" },
      { key: "othercfg_print_tubecodey", label: "Tube code Y (mm)", type: "number", default: "20" },
      { key: "othercfg_print_tubecodef", label: "Tube code font", type: "select", options: BARCODE_FONT_OPTIONS, default: "A" },
      { key: "othercfg_print_tubecodeo", label: "Tube code orientation", type: "select", options: BARCODE_ORIENTATION, default: "N" },
      { key: "othercfg_print_tubecodeh", label: "Tube code height (dots)", type: "number", default: "18" },
      { key: "othercfg_print_tubecodew", label: "Tube code width (dots)", type: "number", default: "16" },
      { key: "othercfg_print_tubecode_boxw", label: "Tube box width (mm)", type: "number", default: "4" },
      { key: "othercfg_print_tubecode_boxh", label: "Tube box height (mm)", type: "number", default: "4" },
      { key: "othercfg_print_tubecode_boxt", label: "Tube box border (dot)", type: "number", default: "1" },
      { key: "othercfg_print_tubecode_boxc", label: "Tube box color", type: "select", options: BARCODE_BOX_COLOR, default: "B" },
      { key: "othercfg_print_tubecode_boxr", label: "Tube box radius", type: "number", default: "4" },
    ],
  },
  {
    title: "Local HTTP server",
    fields: [
      { key: "local_http_address", label: "IP local si port", type: "text", default: "127.0.0.1:18080", span: 2 },
      { key: "local_http_language", label: "Limba interfata", type: "select", options: [["ro", "Romana"], ["en", "English"]], default: "ro" },
      { key: "local_http_tls", label: "HTTPS local", type: "select", options: [["false", "Nu"], ["true", "Da"]], default: "false" },
      { key: "local_http_cors_allowed_origins", label: "Origin-uri CORS permise", type: "text", default: "https://ldse.wisemed.eu", span: 2 },
    ],
  },
];

const state = {
  session: null,
  analytes: [],
  qcTargets: [],
  qcLevels: [],
  selectedAnalyteId: null,
  selectedTag: null,
  selectedQCTargetAnalyteTag: null,
  currentView: "overview",
  language: "ro",
  readerInfo: null,
  layoutKind: "simple_list",
  commType: "file",
  orders: [],
  qcRecords: [],
  rounds: [1],
  qcRounds: [1],
  selectedRoundNo: 1,
  selectedOrderDate: localISODate(),
  selectedQCRoundNo: 1,
  selectedQCPeriod: "current_month",
  selectedQCDate: localISODate(),
  selectedQCDateFrom: offsetISODate(-30),
  selectedQCDateTo: localISODate(),
  selectedQCLevel: "",
  selectedQCAnalyteFilter: "",
  selectedQCTargetID: null,
  editingQCTargetID: null,
  westgardMetrics: null,
  qcWestgardMetrics: null,
  qcWestgardPeriod: "current_month",
  selectedOrderId: null,
  selectedQCRecordId: null,
  selectedQCAnalysisId: null,
  selectedQCAnalysisTag: null,
  selectedOrderAnalysisID: null,
  selectedOrderIDs: [],
  settingsSubView: "reader",
  dailyDetailDefinitions: [],
  dailyDetailValues: [],
  selectedDailyDetailDefinitionID: null,
  selectedDailyDetailDefinitionKey: null,
  dailyDetailsDate: localISODate(),
  dailyDetailsRounds: [1],
  dailyDetailsScopeTab: "day",
  dailyDetailsRoundNo: 1,
  dailyDetailsAnalyteTag: "",
  logPollSeconds: 1,
  logPollTimer: null,
  statusPollSeconds: 2,
  statusPollTimer: null,
  toastTimer: null,
  barcodeMode: false,
  utilityMode: false,
  barcodeSettings: {},
  barcodePrinters: [],
  readerSettings: { repeat_mode: "individual" },
  appUpdateSettings: {},
  appUpdateStatus: null,
  wisemedSetup: { settings: {}, configured: false, setup_complete: false, equipment_registered: false },
  wisemedBootstrap: { medical_units: [], analyzer_types: [] },
};

const els = {
  loginView: document.getElementById("login-view"),
  dashboardView: document.getElementById("dashboard-view"),
  loginForm: document.getElementById("login-form"),
  loginError: document.getElementById("login-error"),
  loginSetupHost: document.getElementById("login-setup-host"),
  languageSelectLogin: document.getElementById("language-select-login"),
  languageSelectDashboard: document.getElementById("language-select-dashboard"),
  readerTitle: document.getElementById("reader-title"),
  readerSubtitle: document.getElementById("reader-subtitle"),
  loginAppVersionLink: document.getElementById("login-app-version-link"),
  loginAppUpdateIndicator: document.getElementById("login-app-update-indicator"),
  loginAppUpdateMessage: document.getElementById("login-app-update-message"),
  dashboardAppVersionLink: document.getElementById("dashboard-app-version-link"),
  dashboardAppUpdateIndicator: document.getElementById("dashboard-app-update-indicator"),
  dashboardAppUpdateMessage: document.getElementById("dashboard-app-update-message"),
  readerIdentityLabel: document.getElementById("reader-identity-label"),
  readerIdentityBadge: document.getElementById("reader-identity-badge"),
  readerIdentityIcon: document.getElementById("reader-identity-icon"),
  readerIdentityTitle: document.getElementById("reader-identity-title"),
  readerIdentitySubtitle: document.getElementById("reader-identity-subtitle"),
  readerSummaryTitle: document.getElementById("reader-summary-title"),
  readerSummaryList: document.getElementById("reader-summary-list"),
  sessionUser: document.getElementById("session-user"),
  statusCards: document.getElementById("status-cards"),
  wsDot: document.getElementById("ws-dot"),
  analyzerDot: document.getElementById("analyzer-dot"),
  wisemedwsPill: document.getElementById("wisemedws-pill"),
  analyzerPill: document.getElementById("analyzer-pill"),
  appToast: document.getElementById("app-toast"),
  appToastBody: document.getElementById("app-toast-body"),
  appToastClose: document.getElementById("app-toast-close"),
  wisemedwsStatusLabel: document.getElementById("wisemedws-status-label"),
  analyzerStatusLabel: document.getElementById("analyzer-status-label"),
  logsList: document.getElementById("logs-list"),
  logPollSeconds: document.getElementById("log-poll-seconds"),
  refreshLogsBtn: document.getElementById("refresh-logs"),
  settingsPanelReader: document.getElementById("settings-panel-reader"),
  readerSettingsForm: document.getElementById("reader-settings-form"),
  repeatModeSelect: document.getElementById("repeat-mode-select"),
  runResultSyncBtn: document.getElementById("run-result-sync"),
  resetResultSyncBtn: document.getElementById("reset-result-sync"),
  resultSyncStatus: document.getElementById("result-sync-status"),
  readerSettingsMessage: document.getElementById("reader-settings-message"),
  analyteList: document.getElementById("analyte-list"),
  settingsSubmenu: document.getElementById("settings-submenu"),
  settingsPanelAnalytes: document.getElementById("settings-panel-analytes"),
  settingsPanelQCEditor: document.getElementById("settings-panel-qc-editor"),
  settingsPanelDailyDetails: document.getElementById("settings-panel-daily-details"),
  analyteForm: document.getElementById("analyte-form"),
  analyteMessage: document.getElementById("analyte-message"),
  deleteAnalyteBtn: document.getElementById("delete-analyte"),
  newAnalyteBtn: document.getElementById("new-analyte"),
  refreshAnalytesBtn: document.getElementById("refresh-analytes"),
  analyteModal: document.getElementById("analyte-modal"),
  analyteModalBackdrop: document.getElementById("analyte-modal-backdrop"),
  closeAnalyteModalBtn: document.getElementById("close-analyte-modal"),
  analyteSearch: document.getElementById("analyte-search"),
  refreshOrdersBtn: document.getElementById("refresh-orders"),
  orderDate: document.getElementById("order-date"),
  roundSelect: document.getElementById("round-select"),
  ordersSelectAll: document.getElementById("orders-select-all"),
  ordersSelectAllBox: document.getElementById("orders-select-all-box"),
  ordersSelectAllLabel: document.getElementById("orders-select-all-label"),
  importOrdersBtn: document.getElementById("import-orders"),
  exportOrdersBtn: document.getElementById("export-orders"),
  worklistOrdersBtn: document.getElementById("worklist-orders"),
  newRoundBtn: document.getElementById("new-round"),
  ordersImportFile: document.getElementById("orders-import-file"),
  ordersLayout: document.getElementById("orders-layout"),
  orderDetails: document.getElementById("order-details"),
  dailyDetailsDate: document.getElementById("daily-details-date"),
  dailyDetailsRoundBox: document.getElementById("daily-details-round-box"),
  dailyDetailsRound: document.getElementById("daily-details-round"),
  dailyDetailsAnalyteBox: document.getElementById("daily-details-analyte-box"),
  dailyDetailsAnalyte: document.getElementById("daily-details-analyte"),
  dailyDetailsValueSearch: document.getElementById("daily-details-value-search"),
  refreshDailyDetailValuesBtn: document.getElementById("refresh-daily-detail-values"),
  dailyDetailsTabs: [...document.querySelectorAll("[data-daily-scope]")],
  dailyDetailDefinitionSearch: document.getElementById("daily-detail-search"),
  dailyDetailDefinitionList: document.getElementById("daily-detail-definition-list"),
  newDailyDetailDefinitionBtn: document.getElementById("new-daily-detail-definition"),
  refreshDailyDetailDefinitionsBtn: document.getElementById("refresh-daily-detail-definitions"),
  dailyDetailDefinitionModal: document.getElementById("daily-detail-definition-modal"),
  dailyDetailDefinitionModalBackdrop: document.getElementById("daily-detail-definition-modal-backdrop"),
  closeDailyDetailDefinitionModalBtn: document.getElementById("close-daily-detail-definition-modal"),
  dailyDetailDefinitionForm: document.getElementById("daily-detail-definition-form"),
  dailyDetailDefinitionMessage: document.getElementById("daily-detail-definition-message"),
  deleteDailyDetailDefinitionBtn: document.getElementById("delete-daily-detail-definition"),
  refreshQCBtn: document.getElementById("refresh-qc"),
  qcPeriod: document.getElementById("qc-period"),
  qcDateFrom: document.getElementById("qc-date-from"),
  qcDateTo: document.getElementById("qc-date-to"),
  newQCRecordBtn: document.getElementById("new-qc-record"),
  qcRoundSelect: document.getElementById("qc-round-select"),
  qcAnalyteFilter: document.getElementById("qc-analyte-filter"),
  qcLevelFilter: document.getElementById("qc-level-filter"),
  qcLayout: document.getElementById("qc-layout"),
  qcDetails: document.getElementById("qc-details"),
  qcSummary: document.getElementById("qc-summary"),
  qcOpenWestgardBtn: document.getElementById("qc-open-westgard"),
  qcRecordModal: document.getElementById("qc-record-modal"),
  qcRecordModalBackdrop: document.getElementById("qc-record-modal-backdrop"),
  closeQCRecordModalBtn: document.getElementById("close-qc-record-modal"),
  qcRecordForm: document.getElementById("qc-record-form"),
  qcRecordMessage: document.getElementById("qc-record-message"),
  qcRecordDate: document.getElementById("qc-record-date"),
  qcRecordAnalyte: document.getElementById("qc-record-analyte"),
  qcRecordLot: document.getElementById("qc-record-lot"),
  qcRecordLevel: document.getElementById("qc-record-level"),
  qcRecordLabel: document.getElementById("qc-record-label"),
  qcRecordQualitative: document.getElementById("qc-record-qualitative"),
  qcRecordQuantitative: document.getElementById("qc-record-quantitative"),
  qcRecordInterpretation: document.getElementById("qc-record-interpretation"),
  qcWestgardModal: document.getElementById("qc-westgard-modal"),
  qcWestgardModalBackdrop: document.getElementById("qc-westgard-modal-backdrop"),
  closeQCWestgardModalBtn: document.getElementById("close-qc-westgard-modal"),
  qcWestgardPeriod: document.getElementById("qc-westgard-period"),
  qcWestgardDateFrom: document.getElementById("qc-westgard-date-from"),
  qcWestgardDateTo: document.getElementById("qc-westgard-date-to"),
  qcWestgardValidateBtn: document.getElementById("qc-westgard-validate"),
  qcWestgardSummary: document.getElementById("qc-westgard-summary"),
  qcWestgardChart: document.getElementById("qc-westgard-chart"),
  qcWestgardRules: document.getElementById("qc-westgard-rules"),
  qcTargetForm: document.getElementById("qc-target-form"),
  qcTargetModal: document.getElementById("qc-target-modal"),
  qcTargetModalBackdrop: document.getElementById("qc-target-modal-backdrop"),
  closeQCTargetModalBtn: document.getElementById("close-qc-target-modal"),
  qcTargetMessage: document.getElementById("qc-target-message"),
  qcTargetAnalyte: document.getElementById("qc-target-analyte"),
  qcTargetFilterAnalyte: document.getElementById("qc-target-filter-analyte"),
  qcTargetLevel: document.getElementById("qc-target-level"),
  qcTargetLot: document.getElementById("qc-target-lot"),
  qcTargetMean: document.getElementById("qc-target-mean"),
  qcTargetSD: document.getElementById("qc-target-sd"),
  qcTarget1SD: document.getElementById("qc-target-1sd"),
  qcTarget2SD: document.getElementById("qc-target-2sd"),
  qcTarget3SD: document.getElementById("qc-target-3sd"),
  qcTargetCV: document.getElementById("qc-target-cv"),
  deleteQCTargetBtn: document.getElementById("delete-qc-target"),
  qcTargetList: document.getElementById("qc-target-list"),
  newQCTargetBtn: document.getElementById("new-qc-target"),
  refreshQCTargetsBtn: document.getElementById("refresh-qc-targets"),
  todayDonut: document.getElementById("today-donut"),
  todayLegend: document.getElementById("today-legend"),
  lineChart: document.getElementById("line-chart"),
  lineLegend: document.getElementById("line-legend"),
  logoutBtn: document.getElementById("logout-btn"),
  logsPanel: document.getElementById("logs-panel"),
  navLinks: [...document.querySelectorAll(".nav-link")],
  navSublinks: [...document.querySelectorAll("#settings-submenu .nav-sublink")],
  views: {
    overview: document.getElementById("view-overview"),
    analytes: document.getElementById("view-analytes"),
    "daily-details": document.getElementById("view-daily-details"),
    orders: document.getElementById("view-orders"),
    qc: document.getElementById("view-qc"),
    help: document.getElementById("view-help"),
  },
};

init();

async function init() {
  bindEvents();
  const prefResp = await api("/api/preferences");
  state.language = prefResp.preferences?.language || "ro";
  syncLanguageControls();
  applyLanguage();
  const sessionResp = await api("/api/session");
  if (sessionResp.preferences?.language) {
    state.language = sessionResp.preferences.language;
    syncLanguageControls();
    applyLanguage();
  }
  state.readerInfo = { ...(sessionResp.reader || {}) };
  state.appUpdateSettings = { ...(sessionResp.app_update || {}) };
  syncAppVersionUI();
  refreshAppUpdateStatus(false).catch(() => {});
  if (sessionResp.authenticated) {
    state.session = sessionResp.session;
    await mountDashboard(sessionResp);
    return;
  }
  state.wisemedSetup = sessionResp.wisemed || state.wisemedSetup;
  showLogin();
}

function bindEvents() {
  els.loginForm.addEventListener("submit", onLogin);
  els.logoutBtn.addEventListener("click", onLogout);
  els.languageSelectLogin.addEventListener("change", onLanguageChange);
  els.languageSelectDashboard.addEventListener("change", onLanguageChange);
  if (els.loginAppVersionLink) els.loginAppVersionLink.addEventListener("click", onAppVersionClick);
  if (els.dashboardAppVersionLink) els.dashboardAppVersionLink.addEventListener("click", onAppVersionClick);
  if (els.loginAppUpdateIndicator) els.loginAppUpdateIndicator.addEventListener("click", onAppUpdateIndicatorClick);
  if (els.dashboardAppUpdateIndicator) els.dashboardAppUpdateIndicator.addEventListener("click", onAppUpdateIndicatorClick);
  els.refreshLogsBtn.addEventListener("click", loadLogs);
  els.logPollSeconds.addEventListener("change", onLogPollChange);
  els.refreshOrdersBtn.addEventListener("click", onRefreshOrdersClick);
  els.orderDate.addEventListener("change", onOrderDateChange);
  els.roundSelect.addEventListener("change", onRoundChange);
  els.ordersSelectAll.addEventListener("change", onOrdersSelectAllChange);
  els.importOrdersBtn.addEventListener("click", onImportOrdersClick);
  els.exportOrdersBtn.addEventListener("click", onExportOrdersClick);
  els.worklistOrdersBtn.addEventListener("click", onWorklistOrdersClick);
  els.newRoundBtn.addEventListener("click", onNewRoundClick);
  els.refreshQCBtn.addEventListener("click", loadQCRecords);
  els.newQCRecordBtn.addEventListener("click", openQCRecordModal);
  if (els.qcPeriod) els.qcPeriod.addEventListener("change", onQCPeriodChange);
  if (els.qcDateFrom) els.qcDateFrom.addEventListener("change", onQCDateRangeChange);
  if (els.qcDateTo) els.qcDateTo.addEventListener("change", onQCDateRangeChange);
  els.qcRoundSelect.addEventListener("change", onQCRoundChange);
  els.qcAnalyteFilter.addEventListener("change", onQCAnalyteFilterChange);
  els.qcLevelFilter.addEventListener("change", onQCLevelFilterChange);
  els.ordersImportFile.addEventListener("change", onOrdersImportFileChange);
  els.appToastClose.addEventListener("click", hideToast);
  els.newAnalyteBtn.addEventListener("click", () => openAnalyteModal());
  els.refreshAnalytesBtn.addEventListener("click", onRefreshAnalytesClick);
  if (els.readerSettingsForm) els.readerSettingsForm.addEventListener("submit", onSaveReaderSettings);
  if (els.runResultSyncBtn) els.runResultSyncBtn.addEventListener("click", onRunResultSync);
  if (els.resetResultSyncBtn) els.resetResultSyncBtn.addEventListener("click", onResetResultSync);
  els.deleteAnalyteBtn.addEventListener("click", onDeleteAnalyte);
  els.analyteForm.addEventListener("submit", onSaveAnalyte);
  els.analyteSearch.addEventListener("input", renderAnalyteList);
  if (els.dailyDetailDefinitionForm) els.dailyDetailDefinitionForm.addEventListener("submit", onSaveDailyDetailDefinition);
  if (els.dailyDetailDefinitionSearch) els.dailyDetailDefinitionSearch.addEventListener("input", renderDailyDetailDefinitionList);
  if (els.newDailyDetailDefinitionBtn) els.newDailyDetailDefinitionBtn.addEventListener("click", () => openDailyDetailDefinitionModal());
  if (els.refreshDailyDetailDefinitionsBtn) els.refreshDailyDetailDefinitionsBtn.addEventListener("click", onRefreshDailyDetailDefinitionsClick);
  if (els.dailyDetailsDate) els.dailyDetailsDate.addEventListener("change", onDailyDetailsDateChange);
  if (els.dailyDetailsRound) els.dailyDetailsRound.addEventListener("change", onDailyDetailsRoundChange);
  if (els.dailyDetailsAnalyte) els.dailyDetailsAnalyte.addEventListener("change", onDailyDetailsAnalyteChange);
  if (els.dailyDetailsValueSearch) els.dailyDetailsValueSearch.addEventListener("input", renderDailyDetailValuesWorkspace);
  if (els.refreshDailyDetailValuesBtn) els.refreshDailyDetailValuesBtn.addEventListener("click", () => loadDailyDetailsWorkspace().catch((error) => showToast(error?.message || "Cannot load daily details", "error")));
  els.dailyDetailsTabs.forEach((tab) => tab.addEventListener("click", () => onDailyDetailsScopeChange(tab.dataset.dailyScope || "day")));
  if (els.dailyDetailDefinitionModalBackdrop) els.dailyDetailDefinitionModalBackdrop.addEventListener("click", closeDailyDetailDefinitionModal);
  if (els.closeDailyDetailDefinitionModalBtn) els.closeDailyDetailDefinitionModalBtn.addEventListener("click", closeDailyDetailDefinitionModal);
  if (els.deleteDailyDetailDefinitionBtn) els.deleteDailyDetailDefinitionBtn.addEventListener("click", onDeleteDailyDetailDefinition);
  els.qcTargetForm.addEventListener("submit", onSaveQCTarget);
  els.newQCTargetBtn.addEventListener("click", async () => {
    if (!state.qcLevels.length) {
      await loadQCMeta().catch(() => {});
    }
    openQCTargetModal();
  });
  els.refreshQCTargetsBtn.addEventListener("click", onRefreshQCTargetsClick);
  els.deleteQCTargetBtn.addEventListener("click", onDeleteQCTarget);
  els.qcTargetFilterAnalyte.addEventListener("change", onQCTargetFilterAnalyteChange);
  els.qcTargetAnalyte.addEventListener("change", onQCTargetAnalyteChange);
  els.qcTargetLevel.addEventListener("change", syncSelectedTargetFromForm);
  els.qcTargetLot.addEventListener("input", syncSelectedTargetFromForm);
  els.qcTargetMean.addEventListener("input", updateQCTargetDerivedFields);
  els.qcTargetSD.addEventListener("input", updateQCTargetDerivedFields);
  els.qcOpenWestgardBtn.addEventListener("click", onGenerateQCWestgard);
  els.qcRecordModalBackdrop.addEventListener("click", closeQCRecordModal);
  els.closeQCRecordModalBtn.addEventListener("click", closeQCRecordModal);
  els.qcRecordForm.addEventListener("submit", onSaveQCRecord);
  els.qcRecordAnalyte.addEventListener("change", syncQCRecordLotOptions);
  els.qcRecordLot.addEventListener("change", syncQCRecordLotMetadata);
  els.qcWestgardModalBackdrop.addEventListener("click", closeQCWestgardModal);
  els.closeQCWestgardModalBtn.addEventListener("click", closeQCWestgardModal);
  els.qcWestgardPeriod.addEventListener("change", onQCWestgardPeriodChange);
  els.qcWestgardDateFrom.addEventListener("change", onQCWestgardCustomDateChange);
  els.qcWestgardDateTo.addEventListener("change", onQCWestgardCustomDateChange);
  els.qcWestgardValidateBtn.addEventListener("click", onShowQCWestgardIssues);
  els.qcTargetModalBackdrop.addEventListener("click", closeQCTargetModal);
  els.closeQCTargetModalBtn.addEventListener("click", closeQCTargetModal);
  els.analyteModalBackdrop.addEventListener("click", closeAnalyteModal);
  els.closeAnalyteModalBtn.addEventListener("click", closeAnalyteModal);
  els.navLinks.forEach((link) => link.addEventListener("click", () => {
    if (link.dataset.view === "analytes") {
      state.settingsSubView = "reader";
    }
    activateView(link.dataset.view);
  }));
  els.navSublinks.forEach((link) => link.addEventListener("click", () => {
    activateSettingsSubView(link.dataset.settingsSubview || "reader");
    activateView(link.dataset.view || "analytes");
  }));
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && !els.analyteModal.hidden) {
      closeAnalyteModal();
      return;
    }
    if (els.dailyDetailDefinitionModal && !els.dailyDetailDefinitionModal.hidden && event.key === "Escape") {
      closeDailyDetailDefinitionModal();
      return;
    }
    if (event.key === "Escape" && !els.qcTargetModal.hidden) {
      closeQCTargetModal();
      return;
    }
    if (event.key === "Escape" && !els.qcRecordModal.hidden) {
      closeQCRecordModal();
      return;
    }
    if (event.key === "Escape" && !els.qcWestgardModal.hidden) {
      closeQCWestgardModal();
    }
  });
  window.addEventListener("popstate", () => {
    activateView(viewFromLocation(), false);
  });
}

function isBarcodeMode() {
  const code = String(state.readerInfo?.analyzer_code || "").toLowerCase();
  const readerID = String(state.readerInfo?.id || "").toLowerCase();
  const protocol = String(state.readerInfo?.protocol || "").toLowerCase();
  return code === "barcodeprinter" || protocol === "barcodeprinter" || readerID.includes("barcodeprinter");
}

function isUtilityMode() {
  const commType = String(state.readerInfo?.comm_type || state.readerSettings?.analyzer_comm_type || state.commType || "").toLowerCase();
  return commType === "utility";
}

function barcodeNavButton(viewName) {
  return document.querySelector(`.nav-link[data-view="${viewName}"]`);
}

function applyBarcodeModeUI() {
  state.barcodeMode = isBarcodeMode();
  state.utilityMode = isUtilityMode();
  if (!state.utilityMode) return;

  const navOrders = barcodeNavButton("orders");
  const navQC = barcodeNavButton("qc");
  const navDailyDetails = barcodeNavButton("daily-details");
  if (navOrders && state.barcodeMode) navOrders.textContent = state.language === "en" ? "History" : "Istoric";
  if (navQC) navQC.style.display = "none";
  if (state.barcodeMode) {
    if (navDailyDetails) navDailyDetails.style.display = "none";
    if (els.settingsSubmenu) els.settingsSubmenu.hidden = true;
    if (els.settingsPanelReader) els.settingsPanelReader.hidden = true;
    if (els.settingsPanelQCEditor) els.settingsPanelQCEditor.hidden = true;
  }

  if (state.barcodeMode) {
    if (els.importOrdersBtn) els.importOrdersBtn.hidden = true;
    if (els.exportOrdersBtn) els.exportOrdersBtn.hidden = true;
    if (els.worklistOrdersBtn) els.worklistOrdersBtn.hidden = true;
    if (els.newRoundBtn) els.newRoundBtn.hidden = true;
    if (els.ordersSelectAllBox) els.ordersSelectAllBox.hidden = true;
    if (els.orderDate?.parentElement) els.orderDate.parentElement.hidden = true;
    if (els.roundSelect?.parentElement) els.roundSelect.parentElement.hidden = true;
    if (els.qcSummary?.closest(".panel")) els.qcSummary.closest(".panel").style.display = "none";
  }
}

async function onLanguageChange(event) {
  const lang = event.target.value;
  const resp = await api("/api/preferences/language", {
    method: "PUT",
    body: JSON.stringify({ language: lang }),
  });
  state.language = resp.preferences.language;
  syncLanguageControls();
  applyLanguage();
  if (!els.dashboardView.hidden) {
    if (state.barcodeMode) {
      await Promise.all([loadStatus(), loadLogs(), loadDashboard(), loadBarcodeSettingsView(), loadBarcodeHistory()]);
    } else if (state.utilityMode) {
      await Promise.all([loadStatus(), loadLogs(), loadReaderSettings(), loadAnalytes(), loadDailyDetailDefinitions(), loadOrders(), loadDashboard()]);
    } else {
      await Promise.all([loadStatus(), loadLogs(), loadAnalytes(), loadQCTargets(), loadDailyDetailDefinitions(), loadOrders(), loadQCRecords(), loadDashboard()]);
      if (state.currentView === "daily-details" || state.settingsSubView === "daily-details") {
        await loadDailyDetailsWorkspace();
      }
    }
  }
}

async function onLogin(event) {
  event.preventDefault();
  els.loginError.hidden = true;
  try {
    const latestSetup = await api("/api/wisemed/setup");
    state.wisemedSetup = latestSetup;
  } catch (error) {
    els.loginError.hidden = false;
    els.loginError.textContent = error.message;
    return;
  }
  if (!state.wisemedSetup?.setup_complete) {
    els.loginError.hidden = false;
    els.loginError.textContent = "Completeaza mai intai configurarea WiseMED.";
    return;
  }
  const form = new FormData(els.loginForm);
  try {
    const resp = await api("/api/session/login", {
      method: "POST",
      body: JSON.stringify({
        username: form.get("username"),
        password: form.get("password"),
        medical_unit_id: state.wisemedSetup?.settings?.unitate_medicala_id || "",
      }),
    });
    state.session = resp.session;
    await mountDashboard(await api("/api/session"));
  } catch (error) {
    els.loginError.hidden = false;
    els.loginError.textContent = error.message;
  }
}

async function onLogout() {
  await api("/api/session/logout", { method: "POST" });
  state.session = null;
  state.orders = [];
  state.selectedOrderId = null;
  els.loginForm.reset();
  els.loginError.hidden = true;
  if (state.logPollTimer) {
    clearInterval(state.logPollTimer);
    state.logPollTimer = null;
  }
  if (state.statusPollTimer) {
    clearInterval(state.statusPollTimer);
    state.statusPollTimer = null;
  }
  showLogin();
}

function showLogin() {
  els.loginView.hidden = false;
  els.dashboardView.hidden = true;
  hideAppUpdateMessage();
  renderLoginSetupPanel().catch((error) => {
    els.loginError.hidden = false;
    els.loginError.textContent = error.message || "Nu s-au putut incarca setarile WiseMED.";
  });
  syncLanguageControls();
  applyLanguage();
}

async function renderLoginSetupPanel() {
  if (!els.loginSetupHost) return;
  const setupResp = await api("/api/wisemed/setup");
  state.wisemedSetup = setupResp;
  const needsSetup = !setupResp.setup_complete;
  if (!needsSetup) {
    els.loginSetupHost.innerHTML = "";
    return;
  }
  let bootstrap = { medical_units: [], analyzer_types: [] };
  if (setupResp.configured) {
    try {
      bootstrap = await api("/api/wisemed/bootstrap");
    } catch (error) {
      bootstrap = { medical_units: [], analyzer_types: [] };
    }
  }
  state.wisemedBootstrap = bootstrap;
  const settings = { ...(setupResp.settings || {}) };
  const medicalUnits = bootstrap.medical_units || [];
  const analyzerTypes = bootstrap.analyzer_types || [];
  const muOptions = [`<option value="">Selecteaza unitatea medicala</option>`]
    .concat(medicalUnits.map((item) => {
      const value = String(item.medical_unit_id ?? item.id ?? "");
      const label = String(item.medical_unit_name ?? item.name ?? value);
      return `<option value="${escapeHtml(value)}" ${value === String(settings.unitate_medicala_id || "") ? "selected" : ""}>${escapeHtml(label)}</option>`;
    })).join("");
  const typeOptions = [`<option value="">Selecteaza tipul echipamentului</option>`]
    .concat(analyzerTypes.map((item) => {
      const value = String(item.analyzer_type_id ?? item.id ?? "");
      const label = String(item.analyzer_type_name ?? item.name ?? value);
      return `<option value="${escapeHtml(value)}" ${value === String(settings.tip_de_echipament_id || "") ? "selected" : ""}>${escapeHtml(label)}</option>`;
    })).join("");
  els.loginSetupHost.innerHTML = `
    <section class="login-setup-panel">
      <div class="panel-head">
        <h3>Initializare WiseMED</h3>
        <div class="inline-actions">
          <button id="wisemed-setup-refresh" type="button" class="ghost">Refresh</button>
        </div>
      </div>
      <p class="small muted">${needsSetup ? "Completeaza datele de conectare, apoi autentifica-te cu utilizatorul WiseMED." : "Configurarea WiseMED este disponibila local si poate fi actualizata oricand."}</p>
      <form id="wisemed-setup-form" class="stack">
        <div class="two-col">
          <label><span>Protocol</span><select name="cfg_wisemed_protocol"><option value="https" ${String(settings.cfg_wisemed_protocol || "") === "https" ? "selected" : ""}>https</option><option value="http" ${String(settings.cfg_wisemed_protocol || "") === "http" ? "selected" : ""}>http</option></select></label>
          <label><span>IP / Host</span><input name="cfg_wisemed_ip" value="${escapeHtml(settings.cfg_wisemed_ip || "")}"></label>
        </div>
        <div class="two-col">
          <label><span>Port</span><input name="cfg_wisemed_port" value="${escapeHtml(settings.cfg_wisemed_port || "")}"></label>
          <label><span>Path</span><input name="cfg_wisemed_path" value="${escapeHtml(settings.cfg_wisemed_path || "/api")}"></label>
        </div>
        <label><span>API key</span><input name="cfg_wisemed_key" value="${escapeHtml(settings.cfg_wisemed_key || "")}"></label>
        <div class="two-col">
          <label><span>Unitate medicala</span><select name="unitate_medicala_id">${muOptions}</select></label>
          <label><span>Tip echipament</span><select name="tip_de_echipament_id">${typeOptions}</select></label>
        </div>
        <div class="two-col">
          <label><span>Cod echipament</span><input name="cod_echipament" value="${escapeHtml(settings.cod_echipament || "")}"></label>
          <label><span>Serie echipament</span><input name="numar_serial_echipament" value="${escapeHtml(settings.numar_serial_echipament || "")}"></label>
        </div>
        <div class="two-col">
          <label><span>Echipament ID</span><input value="${escapeHtml(settings.echipament_id || "")}" readonly></label>
          <label><span>API key echipament</span><input value="${escapeHtml(settings.api_key_echipament || "")}" readonly></label>
        </div>
        <div class="orders-buttons">
          <button id="wisemed-setup-save" type="submit" class="ghost">Salveaza configurarea</button>
        </div>
        <p id="wisemed-setup-message" class="small muted"></p>
      </form>
    </section>
  `;

  const form = document.getElementById("wisemed-setup-form");
  const msg = document.getElementById("wisemed-setup-message");
  const refreshBtn = document.getElementById("wisemed-setup-refresh");
  if (refreshBtn) {
    refreshBtn.addEventListener("click", () => {
      renderLoginSetupPanel().catch((error) => {
        els.loginError.hidden = false;
        els.loginError.textContent = error.message || "Refresh WiseMED failed.";
      });
    });
  }
  if (form) {
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      msg.textContent = "";
      const payload = Object.fromEntries(new FormData(form).entries());
      const saved = await api("/api/wisemed/setup", {
        method: "PUT",
        body: JSON.stringify(payload),
      });
      state.wisemedSetup = saved;
      msg.textContent = "Configurarea WiseMED a fost salvata.";
      await renderLoginSetupPanel();
    });
  }
}

async function mountDashboard(sessionResp) {
  els.loginView.hidden = true;
  els.dashboardView.hidden = false;
  state.readerInfo = { ...(sessionResp.reader || {}) };
  state.appUpdateSettings = { ...(sessionResp.app_update || state.appUpdateSettings || {}) };
  state.settingsSubView = settingsSubViewFromLocation();
  applyBarcodeModeUI();
  els.readerTitle.textContent = sessionResp.reader.label;
  els.readerSubtitle.textContent = `${sessionResp.reader.analyzer_name} · ${sessionResp.reader.id}`;
  syncAppVersionUI();
  const userParts = [state.session?.first_name, state.session?.last_name].filter(Boolean);
  els.sessionUser.textContent = userParts.join(" ").trim() || state.session?.username || "-";
  syncLanguageControls();
  applyLanguage();
  toggleLogsAccess(!!sessionResp.permissions?.can_view_logs);
  syncOrderControls();
  if (!state.barcodeMode) {
    activateSettingsSubView(state.settingsSubView || "reader");
  }
  renderRoundSelect();
  activateView(viewFromLocation(), false);
  if (state.barcodeMode) {
    await Promise.all([loadStatus(), loadLogs(), loadDashboard(), loadBarcodeSettingsView(), loadBarcodeHistory()]);
  } else if (state.utilityMode) {
    await Promise.all([loadStatus(), loadLogs(), loadReaderSettings(), loadAnalytes(), loadDailyDetailDefinitions(), loadOrders(), loadDashboard()]);
    if (state.currentView === "daily-details" || state.settingsSubView === "daily-details") {
      await loadDailyDetailsWorkspace();
    }
  } else {
    await Promise.all([loadStatus(), loadLogs(), loadQCMeta(), loadReaderSettings(), loadAnalytes(), loadQCTargets(), loadDailyDetailDefinitions(), loadOrders(), loadQCRecords(), loadDashboard()]);
    if (state.currentView === "daily-details" || state.settingsSubView === "daily-details") {
      await loadDailyDetailsWorkspace();
    }
  }
  restartLogPolling();
  restartStatusPolling();
  refreshAppUpdateStatus(false).catch(() => {});
}

function syncAppVersionUI() {
  const version = String(state.appUpdateSettings?.current_version || state.readerInfo?.app_version || "0.0.0").trim() || "0.0.0";
  const label = `v${version}`;
  if (els.loginAppVersionLink) els.loginAppVersionLink.textContent = label;
  if (els.dashboardAppVersionLink) els.dashboardAppVersionLink.textContent = label;
}

async function refreshAppUpdateStatus(force = false) {
  const suffix = force ? "?refresh=1" : "";
  const resp = await api(`/api/app-update/status${suffix}`);
  state.appUpdateStatus = resp || null;
  renderAppUpdateStatus();
  return resp;
}

function renderAppUpdateStatus() {
  const status = state.appUpdateStatus || {};
  const icon = status.icon || "off";
  const iconText = icon === "ok" ? "✓" : icon === "alert" ? "!" : "?";
  [els.loginAppUpdateIndicator, els.dashboardAppUpdateIndicator].forEach((node) => {
    if (!node) return;
    node.classList.remove("ok", "alert", "off");
    node.classList.add(icon);
    node.textContent = iconText;
    node.title = String(status.message || "");
  });
}

function hideAppUpdateMessage() {
  if (els.loginAppUpdateMessage) {
    els.loginAppUpdateMessage.hidden = true;
    els.loginAppUpdateMessage.textContent = "";
  }
  if (els.dashboardAppUpdateMessage) {
    els.dashboardAppUpdateMessage.hidden = true;
    els.dashboardAppUpdateMessage.textContent = "";
  }
}

function showAppUpdateMessage() {
  const message = String(state.appUpdateStatus?.message || "Nu exista informatii de update.").trim();
  if (!els.dashboardView.hidden) {
    showToast(message, state.appUpdateStatus?.icon === "ok" ? "success" : "error");
    return;
  }
  if (els.loginAppUpdateMessage) {
    els.loginAppUpdateMessage.hidden = false;
    els.loginAppUpdateMessage.textContent = message;
  }
}

async function onAppVersionClick(event) {
  event.preventDefault();
  try {
    const resp = await refreshAppUpdateStatus(true);
    if (els.dashboardView.hidden && els.loginAppUpdateMessage) {
      els.loginAppUpdateMessage.hidden = false;
      els.loginAppUpdateMessage.textContent = String(resp?.message || "Verificare update finalizata.");
      return;
    }
    showToast(String(resp?.message || "Verificare update finalizata."), resp?.icon === "ok" ? "success" : "error");
  } catch (error) {
    if (els.dashboardView.hidden && els.loginAppUpdateMessage) {
      els.loginAppUpdateMessage.hidden = false;
      els.loginAppUpdateMessage.textContent = error.message || "Nu s-a putut verifica update-ul.";
      return;
    }
    showToast(error.message || "Nu s-a putut verifica update-ul.", "error");
  }
}

function onAppUpdateIndicatorClick(event) {
  event.preventDefault();
  showAppUpdateMessage();
}

function activateView(name, pushHistory = true) {
  if (name === "help") {
    window.location.href = "/help/";
    return;
  }
  if (state.utilityMode && name === "qc") {
    name = "orders";
  }
  if (name === "analytes") {
    state.settingsSubView = settingsSubViewFromLocation();
  }
  state.currentView = name;
  els.navLinks.forEach((link) => link.classList.toggle("active", link.dataset.view === name));
  Object.entries(els.views).forEach(([key, view]) => {
    view.hidden = key !== name;
  });
  renderTopbarTitle();
  if (pushHistory) {
    const path = pathForView(name);
    if (window.location.pathname !== path) {
      window.history.pushState({ view: name }, "", path);
    }
  }
  if (name === "analytes") {
    if (state.barcodeMode) {
      loadBarcodeSettingsView().catch(() => {});
    } else {
      activateSettingsSubView(state.settingsSubView || "reader");
      Promise.all([loadQCMeta(), loadReaderSettings(), loadAnalytes(), loadQCTargets(), loadDailyDetailDefinitions()]).catch(() => {});
    }
  }
  if (name === "daily-details") {
    Promise.all([
      loadAnalytes().catch(() => {}),
      loadQCTargets().catch(() => {}),
      loadDailyDetailDefinitions().catch(() => {}),
    ]).finally(() => {
      loadDailyDetailsWorkspace().catch(() => {});
    });
  }
  if (name === "orders") {
    if (state.barcodeMode) {
      loadBarcodeHistory().catch(() => {});
    } else {
      loadOrdersWithFeedback();
    }
  }
  if (!state.utilityMode && name === "qc") {
    loadQCRecords().catch(() => {});
  }
}

function renderTopbarTitle() {
  const title = document.getElementById("dashboard-title");
  if (!title) return;
  if (state.barcodeMode) {
    if (state.currentView === "analytes") {
      title.textContent = t("navSettings");
      return;
    }
    if (state.currentView === "daily-details") {
      title.textContent = t("navDailyDetails");
      return;
    }
    if (state.currentView === "orders") {
      title.textContent = state.language === "en" ? "Print history" : "Istoric tipariri";
      return;
    }
  }
  if (state.currentView === "daily-details") {
    title.textContent = t("navDailyDetails");
    return;
  }
  if (state.currentView === "analytes") {
    title.textContent = state.settingsSubView === "reader"
      ? t("settingsReader")
      : (state.settingsSubView === "qc" ? t("settingsQc") : (state.settingsSubView === "daily-details" ? t("settingsDailyDetails") : t("settingsAnalytes")));
    return;
  }
  if (state.currentView === "orders") {
    title.textContent = t("navOrders");
    return;
  }
  if (state.currentView === "qc") {
    title.textContent = t("navQc");
    return;
  }
  if (state.currentView === "help") {
    title.textContent = t("navHelp");
    return;
  }
  title.textContent = t("dashboardTitle");
}

function viewFromLocation() {
  const path = window.location.pathname || "/";
  if (state.barcodeMode) {
    if (path === "/settings" || path === "/settings/reader" || path === "/settings/analytes" || path === "/settings/qc") return "analytes";
    if (path === "/daily-details") return "daily-details";
    if (path === "/orders") return "orders";
    if (path === "/help") return "help";
    return "overview";
  }
  if (path === "/settings" || path === "/settings/reader" || path === "/settings/analytes" || path === "/settings/qc" || path === "/settings/daily-details") return "analytes";
  if (path === "/daily-details") return "daily-details";
  if (path === "/orders") return "orders";
  if (path === "/qc") return "qc";
  if (path === "/help") return "help";
  return "overview";
}

function settingsSubViewFromLocation() {
  const path = window.location.pathname || "/";
  if (path === "/settings/reader" || path === "/settings") return "reader";
  if (path === "/settings/qc") return "qc";
  if (path === "/settings/daily-details") return "daily-details";
  return "analytes";
}

function pathForView(name) {
  if (state.barcodeMode) {
    if (name === "analytes") return "/settings";
    if (name === "daily-details") return "/daily-details";
    if (name === "orders") return "/orders";
    if (name === "help") return "/help";
    return "/";
  }
  if (name === "analytes") {
    if (state.settingsSubView === "reader") return "/settings/reader";
    if (state.settingsSubView === "qc") return "/settings/qc";
    if (state.settingsSubView === "daily-details") return "/settings/daily-details";
    return "/settings/analytes";
  }
  if (name === "daily-details") return "/daily-details";
  if (name === "orders") return "/orders";
  if (name === "qc") return "/qc";
  if (name === "help") return "/help";
  return "/";
}

function activateSettingsSubView(name) {
  if (state.barcodeMode) {
    state.settingsSubView = "reader";
    if (els.settingsPanelAnalytes) els.settingsPanelAnalytes.hidden = false;
    if (els.settingsPanelReader) els.settingsPanelReader.hidden = true;
    if (els.settingsPanelQCEditor) els.settingsPanelQCEditor.hidden = true;
    if (!els.views.analytes.hidden) {
      renderTopbarTitle();
    }
    return;
  }
  state.settingsSubView = ["reader", "qc", "daily-details", "analytes"].includes(name) ? name : "reader";
  const readerActive = state.settingsSubView === "reader";
  const qcActive = state.settingsSubView === "qc";
  const dailyDetailsActive = state.settingsSubView === "daily-details";
  if (els.settingsPanelReader) els.settingsPanelReader.hidden = !readerActive;
  els.settingsPanelAnalytes.hidden = readerActive || qcActive || dailyDetailsActive;
  els.settingsPanelQCEditor.hidden = !qcActive;
  if (els.settingsPanelDailyDetails) els.settingsPanelDailyDetails.hidden = !dailyDetailsActive;
  els.navSublinks.forEach((link) => {
    link.classList.toggle("active", (link.dataset.settingsSubview || "analytes") === state.settingsSubView);
  });
  if (!els.views.analytes.hidden) {
    renderTopbarTitle();
    const path = pathForView("analytes");
    if (window.location.pathname !== path) {
      window.history.replaceState({ view: "analytes" }, "", path);
    }
  }
  if (qcActive && !state.qcLevels.length) {
    loadQCMeta().catch(() => {});
  }
  if (dailyDetailsActive) {
    renderDailyDetailDefinitionList();
  }
}

async function loadStatus() {
  const resp = await api("/api/status");
  const data = resp.data || resp;
  state.readerInfo = { ...(state.readerInfo || {}), ...(data.reader || {}) };
  const stats = data.stats || {};
  const communication = data.communication || {};
  state.commType = communication.type || state.commType || "file";
  const layout = data.layout || {};
  state.layoutKind = layout.kind || state.layoutKind || "simple_list";
  const connections = data.connections || {};
  updateConnectionPills(connections);
  const cards = state.barcodeMode ? [
    { label: "Tipariri", value: stats.orders ?? 0 },
    { label: "Etichete", value: stats.results ?? 0 },
    { label: t("events"), value: stats.events ?? 0 },
    { label: t("comm"), value: communication.type || "-" },
    { label: t("layout"), value: layout.kind || "-" },
  ] : [
    { label: t("analytes"), value: stats.analytes ?? 0 },
    { label: t("orders"), value: stats.orders ?? 0 },
    { label: t("results"), value: stats.results ?? 0 },
    { label: t("qcRecords"), value: stats.qc_records ?? 0 },
    { label: t("qcResults"), value: stats.qc_results ?? 0 },
    { label: t("qcTargets"), value: stats.qc_targets ?? 0 },
    { label: t("events"), value: stats.events ?? 0 },
    { label: t("comm"), value: communication.type || "-" },
    { label: t("layout"), value: layout.kind || "-" },
  ];
  els.statusCards.innerHTML = cards.map((item) => `<div class="metric"><span class="muted small">${escapeHtml(item.label)}</span><strong>${escapeHtml(String(item.value))}</strong></div>`).join("");
  renderReaderSidebar();
}

async function loadDashboard() {
  if (state.barcodeMode) {
    const today = localISODate();
    const resp = await api(`/api/barcode/stats/daily?date_from=${encodeURIComponent(today)}&date_to=${encodeURIComponent(today)}`);
    const entry = (resp.daily || [])[0] || { prints: 0, labels: 0, ok: 0, fail: 0 };
    renderDonut({ with_result: Number(entry.ok || 0), without_result: Number(entry.fail || 0) });
    const from = offsetISODate(-14);
    const range = await api(`/api/barcode/stats/daily?date_from=${encodeURIComponent(from)}&date_to=${encodeURIComponent(today)}`);
    const series = (range.daily || []).map((item) => ({
      day: item.day,
      orders: Number(item.prints || 0),
      analyses: Number(item.labels || 0),
      analyses_with_result: Number(item.ok || 0),
    })).reverse();
    renderLineChart(series);
    return;
  }
  const resp = await api("/api/dashboard");
  renderDonut(resp.today || { without_result: 0, with_result: 0 });
  renderQCSummary(resp.qc_today || {});
  renderLineChart(resp.series || []);
}

async function loadReaderSettings() {
  if (state.barcodeMode || !els.repeatModeSelect) return;
  const resp = await api("/api/reader-settings");
  state.readerSettings = {
    repeat_mode: String(resp.settings?.repeat_mode || "individual"),
    reader_id: String(resp.settings?.reader_id || ""),
    reader_label: String(resp.settings?.reader_label || ""),
    analyzer_name: String(resp.settings?.analyzer_name || ""),
    analyzer_code: String(resp.settings?.analyzer_code || ""),
    db_name: String(resp.settings?.db_name || ""),
    sqlite_path: String(resp.settings?.sqlite_path || ""),
    local_http_address: String(resp.settings?.local_http_address || ""),
    local_http_language: String(resp.settings?.local_http_language || "ro"),
    local_http_tls: String(resp.settings?.local_http_tls || "false"),
    local_http_cors_allowed_origins: String(resp.settings?.local_http_cors_allowed_origins || "https://ldse.wisemed.eu"),
    analyzer_comm_type: String(resp.settings?.analyzer_comm_type || ""),
    analyzer_protocol: String(resp.settings?.analyzer_protocol || ""),
    app_updates_enabled: String(resp.settings?.app_updates_enabled || "true"),
    app_updates_app_id: String(resp.settings?.app_updates_app_id || ""),
    app_updates_current_version: String(resp.settings?.app_updates_current_version || "0.0.0"),
    app_updates_channel: String(resp.settings?.app_updates_channel || "stable"),
    app_updates_base_url: String(resp.settings?.app_updates_base_url || ""),
    app_updates_auto_download: String(resp.settings?.app_updates_auto_download || "true"),
    app_updates_download_dir: String(resp.settings?.app_updates_download_dir || "./updates"),
    result_sync_enabled: String(resp.settings?.result_sync_enabled || "true"),
    result_sync_interval_minutes: String(resp.settings?.result_sync_interval_minutes || "5"),
    result_sync_sample_prefixes: String(resp.settings?.result_sync_sample_prefixes || ""),
    result_sync_sample_suffixes: String(resp.settings?.result_sync_sample_suffixes || ""),
    result_sync_separators: String(resp.settings?.result_sync_separators || "-"),
    result_sync_qc_prefixes: String(resp.settings?.result_sync_qc_prefixes || ""),
  };
  const form = els.readerSettingsForm;
  if (form) {
    form.elements.reader_id.value = state.readerSettings.reader_id;
    form.elements.reader_label.value = state.readerSettings.reader_label;
    form.elements.analyzer_name.value = state.readerSettings.analyzer_name;
    form.elements.analyzer_code.value = state.readerSettings.analyzer_code;
    form.elements.db_name.value = state.readerSettings.db_name;
    form.elements.sqlite_path.value = state.readerSettings.sqlite_path;
    form.elements.local_http_address.value = state.readerSettings.local_http_address;
    form.elements.local_http_language.value = state.readerSettings.local_http_language;
    form.elements.local_http_tls.value = state.readerSettings.local_http_tls;
    form.elements.local_http_cors_allowed_origins.value = state.readerSettings.local_http_cors_allowed_origins;
    form.elements.analyzer_comm_type.value = state.readerSettings.analyzer_comm_type;
    form.elements.analyzer_protocol.value = state.readerSettings.analyzer_protocol;
    form.elements.app_updates_enabled.value = state.readerSettings.app_updates_enabled;
    form.elements.app_updates_app_id.value = state.readerSettings.app_updates_app_id;
    form.elements.app_updates_current_version.value = state.readerSettings.app_updates_current_version;
    form.elements.app_updates_channel.value = state.readerSettings.app_updates_channel;
    form.elements.app_updates_base_url.value = state.readerSettings.app_updates_base_url;
    form.elements.app_updates_auto_download.value = state.readerSettings.app_updates_auto_download;
    form.elements.app_updates_download_dir.value = state.readerSettings.app_updates_download_dir;
    form.elements.result_sync_enabled.value = state.readerSettings.result_sync_enabled;
    form.elements.result_sync_interval_minutes.value = state.readerSettings.result_sync_interval_minutes;
    form.elements.result_sync_sample_prefixes.value = state.readerSettings.result_sync_sample_prefixes;
    form.elements.result_sync_sample_suffixes.value = state.readerSettings.result_sync_sample_suffixes;
    form.elements.result_sync_separators.value = state.readerSettings.result_sync_separators;
    form.elements.result_sync_qc_prefixes.value = state.readerSettings.result_sync_qc_prefixes;
  }
  els.repeatModeSelect.value = state.readerSettings.repeat_mode;
  await loadResultSyncStatus();
  if (els.readerSettingsMessage) {
    els.readerSettingsMessage.textContent = "";
  }
}

async function onSaveReaderSettings(event) {
  event.preventDefault();
  if (!els.repeatModeSelect) return;
  const form = els.readerSettingsForm;
  const payload = {
    reader_id: String(form.elements.reader_id.value || "").trim(),
    reader_label: String(form.elements.reader_label.value || "").trim(),
    analyzer_name: String(form.elements.analyzer_name.value || "").trim(),
    analyzer_code: String(form.elements.analyzer_code.value || "").trim(),
    db_name: String(form.elements.db_name.value || "").trim(),
    sqlite_path: String(form.elements.sqlite_path.value || "").trim(),
    local_http_address: String(form.elements.local_http_address.value || "").trim(),
    local_http_language: String(form.elements.local_http_language.value || "ro").trim(),
    local_http_tls: String(form.elements.local_http_tls.value || "false").trim(),
    local_http_cors_allowed_origins: String(form.elements.local_http_cors_allowed_origins.value || "https://ldse.wisemed.eu").trim(),
    analyzer_comm_type: String(form.elements.analyzer_comm_type.value || "").trim(),
    analyzer_protocol: String(form.elements.analyzer_protocol.value || "").trim(),
    app_updates_enabled: String(form.elements.app_updates_enabled.value || "true").trim(),
    app_updates_app_id: String(form.elements.app_updates_app_id.value || "").trim(),
    app_updates_current_version: String(form.elements.app_updates_current_version.value || "0.0.0").trim(),
    app_updates_channel: String(form.elements.app_updates_channel.value || "stable").trim(),
    app_updates_base_url: String(form.elements.app_updates_base_url.value || "").trim(),
    app_updates_auto_download: String(form.elements.app_updates_auto_download.value || "true").trim(),
    app_updates_download_dir: String(form.elements.app_updates_download_dir.value || "./updates").trim(),
    result_sync_enabled: String(form.elements.result_sync_enabled.value || "true").trim(),
    result_sync_interval_minutes: String(form.elements.result_sync_interval_minutes.value || "5").trim(),
    result_sync_sample_prefixes: String(form.elements.result_sync_sample_prefixes.value || "").trim(),
    result_sync_sample_suffixes: String(form.elements.result_sync_sample_suffixes.value || "").trim(),
    result_sync_separators: String(form.elements.result_sync_separators.value || "-").trim(),
    result_sync_qc_prefixes: String(form.elements.result_sync_qc_prefixes.value || "").trim(),
    repeat_mode: String(els.repeatModeSelect.value || "individual"),
  };
  const resp = await api("/api/reader-settings", {
    method: "PUT",
    body: JSON.stringify(payload),
  });
  state.readerSettings = {
    repeat_mode: String(resp.settings?.repeat_mode || payload.repeat_mode),
    reader_id: String(resp.settings?.reader_id || payload.reader_id),
    reader_label: String(resp.settings?.reader_label || payload.reader_label),
    analyzer_name: String(resp.settings?.analyzer_name || payload.analyzer_name),
    analyzer_code: String(resp.settings?.analyzer_code || payload.analyzer_code),
    db_name: String(resp.settings?.db_name || payload.db_name),
    sqlite_path: String(resp.settings?.sqlite_path || payload.sqlite_path),
    local_http_address: String(resp.settings?.local_http_address || payload.local_http_address),
    local_http_language: String(resp.settings?.local_http_language || payload.local_http_language),
    local_http_tls: String(resp.settings?.local_http_tls || payload.local_http_tls),
    local_http_cors_allowed_origins: String(resp.settings?.local_http_cors_allowed_origins || payload.local_http_cors_allowed_origins),
    analyzer_comm_type: String(resp.settings?.analyzer_comm_type || payload.analyzer_comm_type),
    analyzer_protocol: String(resp.settings?.analyzer_protocol || payload.analyzer_protocol),
    app_updates_enabled: String(resp.settings?.app_updates_enabled || payload.app_updates_enabled),
    app_updates_app_id: String(resp.settings?.app_updates_app_id || payload.app_updates_app_id),
    app_updates_current_version: String(resp.settings?.app_updates_current_version || payload.app_updates_current_version),
    app_updates_channel: String(resp.settings?.app_updates_channel || payload.app_updates_channel),
    app_updates_base_url: String(resp.settings?.app_updates_base_url || payload.app_updates_base_url),
    app_updates_auto_download: String(resp.settings?.app_updates_auto_download || payload.app_updates_auto_download),
    app_updates_download_dir: String(resp.settings?.app_updates_download_dir || payload.app_updates_download_dir),
    result_sync_enabled: String(resp.settings?.result_sync_enabled || payload.result_sync_enabled),
    result_sync_interval_minutes: String(resp.settings?.result_sync_interval_minutes || payload.result_sync_interval_minutes),
    result_sync_sample_prefixes: String(resp.settings?.result_sync_sample_prefixes || payload.result_sync_sample_prefixes),
    result_sync_sample_suffixes: String(resp.settings?.result_sync_sample_suffixes || payload.result_sync_sample_suffixes),
    result_sync_separators: String(resp.settings?.result_sync_separators || payload.result_sync_separators),
    result_sync_qc_prefixes: String(resp.settings?.result_sync_qc_prefixes || payload.result_sync_qc_prefixes),
  };
  els.repeatModeSelect.value = state.readerSettings.repeat_mode;
  if (els.readerSettingsMessage) {
    els.readerSettingsMessage.textContent = t("readerSettingsSaved");
  }
  state.readerInfo = {
    ...(state.readerInfo || {}),
    id: state.readerSettings.reader_id || state.readerInfo?.id,
    label: state.readerSettings.reader_label || state.readerInfo?.label,
    analyzer_name: state.readerSettings.analyzer_name || state.readerInfo?.analyzer_name,
    analyzer_code: state.readerSettings.analyzer_code || state.readerInfo?.analyzer_code,
    protocol: state.readerSettings.analyzer_protocol || state.readerInfo?.protocol,
    repeat_mode: state.readerSettings.repeat_mode,
    app_version: state.readerSettings.app_updates_current_version || state.readerInfo?.app_version,
  };
  state.appUpdateSettings = {
    enabled: state.readerSettings.app_updates_enabled,
    app_id: state.readerSettings.app_updates_app_id,
    current_version: state.readerSettings.app_updates_current_version,
    channel: state.readerSettings.app_updates_channel,
    base_url: state.readerSettings.app_updates_base_url,
    auto_download: state.readerSettings.app_updates_auto_download,
    download_dir: state.readerSettings.app_updates_download_dir,
  };
  syncAppVersionUI();
  refreshAppUpdateStatus(true).catch(() => {});
  if (state.readerSettings.local_http_language && state.readerSettings.local_http_language !== state.language) {
    state.language = state.readerSettings.local_http_language;
    syncLanguageControls();
    applyLanguage();
  }
  showToast(t("readerSettingsSaved"), "success");
  await loadResultSyncStatus();
}

async function loadResultSyncStatus() {
  if (!els.resultSyncStatus) return;
  try {
    const resp = await api("/api/result-sync/status");
    const parts = [];
    parts.push(resp.running ? "ruleaza" : "idle");
    parts.push(`interval ${resp.interval_minutes || 5} min`);
    if (resp.last_run_at) parts.push(`ultima rulare ${formatDate(resp.last_run_at)}`);
    if (resp.last_summary?.matched !== undefined) {
      parts.push(`matched ${resp.last_summary.matched || 0}/${resp.last_summary.processed || 0}`);
    }
    if (resp.last_error) parts.push(`eroare: ${resp.last_error}`);
    els.resultSyncStatus.textContent = parts.join(" • ");
  } catch (error) {
    els.resultSyncStatus.textContent = error?.message || "Nu se poate incarca statusul sincronizarii";
  }
}

async function onRunResultSync() {
  try {
    const resp = await api("/api/result-sync/run", { method: "POST" });
    await loadResultSyncStatus();
    showToast(`Sync rulat: ${resp.summary?.matched || 0} match-uri`, "success");
  } catch (error) {
    showToast(error?.message || "Nu se poate rula sincronizarea", "error");
  }
}

async function onResetResultSync() {
  try {
    await api("/api/result-sync/reset", { method: "POST" });
    await loadResultSyncStatus();
    showToast("Status sincronizare resetat", "success");
  } catch (error) {
    showToast(error?.message || "Nu se poate reseta sincronizarea", "error");
  }
}

async function loadBarcodeSettingsView() {
  if (!state.barcodeMode || !els.settingsPanelAnalytes) return;
  const [settingsResp, printersResp, updateResp] = await Promise.all([
    api("/api/barcode/settings"),
    api("/api/barcode/printers").catch(() => ({ printers: [] })),
    api("/api/app-update/settings").catch(() => ({ settings: {} })),
  ]);
  state.barcodeSettings = settingsResp.settings || {};
  state.barcodePrinters = printersResp.printers || [];
  state.appUpdateSettings = updateResp.settings || state.appUpdateSettings || {};
  const values = buildBarcodeSettingsState(state.barcodeSettings);
  els.settingsPanelAnalytes.innerHTML = `
    <div class="orders-toolbar">
      <div class="orders-buttons">
        <button id="barcode-settings-save" class="ghost">Salveaza</button>
        <button id="barcode-settings-test" class="ghost">Test print</button>
        <button id="barcode-settings-refresh" class="ghost">Refresh</button>
      </div>
    </div>
    <div class="barcode-settings-layout">
      <div class="table-wrap barcode-settings-form">
        ${BARCODE_FIELD_SECTIONS.map((section) => `
          <section class="barcode-settings-section">
            <h3>${escapeHtml(section.title)}</h3>
            <div class="barcode-settings-grid">
              ${section.fields.map((field) => renderBarcodeField(field, values, state.barcodePrinters)).join("")}
            </div>
          </section>`).join("")}
        <section class="barcode-settings-section">
          <h3>Update server</h3>
          <div class="barcode-settings-grid">
            ${renderBarcodeUpdateField("Update enabled", "enabled", state.appUpdateSettings.enabled || "true", "select", [["true", "Da"], ["false", "Nu"]])}
            ${renderBarcodeUpdateField("App ID update", "app_id", state.appUpdateSettings.app_id || state.readerInfo?.id || "")}
            ${renderBarcodeUpdateField("Versiune curenta", "current_version", state.appUpdateSettings.current_version || state.readerInfo?.app_version || "0.0.0")}
            ${renderBarcodeUpdateField("Canal", "channel", state.appUpdateSettings.channel || "stable", "select", [["stable", "stable"]])}
            ${renderBarcodeUpdateField("Update server URL", "base_url", state.appUpdateSettings.base_url || "", "text", null, 2)}
            ${renderBarcodeUpdateField("Auto download", "auto_download", state.appUpdateSettings.auto_download || "true", "select", [["true", "Da"], ["false", "Nu"]])}
            ${renderBarcodeUpdateField("Director download update", "download_dir", state.appUpdateSettings.download_dir || "./updates", "text", null, 2)}
          </div>
        </section>
      </div>
      <aside class="panel barcode-preview-panel">
        <div class="panel-head">
          <h3>Preview eticheta</h3>
        </div>
        <div id="barcode-preview" class="barcode-preview"></div>
      </aside>
    </div>
  `;

  const collect = () => {
    const out = buildBarcodeSettingsState(state.barcodeSettings);
    els.settingsPanelAnalytes.querySelectorAll("[data-setting-key]").forEach((node) => {
      const key = node.getAttribute("data-setting-key");
      if (!key) return;
      out[key] = node.value ?? "";
    });
    out.othercfg_printer_type = out.bcp_type || "zebrazpl";
    out.othercfg_bc_width = out.othercfg_print_bcodeopt_w || out.othercfg_bc_width || "2";
    return out;
  };
  const collectUpdate = () => {
    const out = {};
    els.settingsPanelAnalytes.querySelectorAll("[data-update-setting-key]").forEach((node) => {
      const key = node.getAttribute("data-update-setting-key");
      if (!key) return;
      out[key] = node.value ?? "";
    });
    return out;
  };

  const saveBtn = document.getElementById("barcode-settings-save");
  const testBtn = document.getElementById("barcode-settings-test");
  const refreshBtn = document.getElementById("barcode-settings-refresh");
  const bindRerender = () => {
    applyBarcodeFieldVisibility();
    renderBarcodeSettingsPreview(collect());
  };

  els.settingsPanelAnalytes.querySelectorAll("[data-setting-key]").forEach((node) => {
    node.addEventListener("input", bindRerender);
    node.addEventListener("change", bindRerender);
  });

  if (saveBtn) {
    saveBtn.addEventListener("click", async () => {
      const payload = collect();
      const updatePayload = collectUpdate();
      const settingsResp = await api("/api/barcode/settings", {
        method: "PUT",
        body: JSON.stringify(payload),
      });
      const updateResp = await api("/api/app-update/settings", {
        method: "PUT",
        body: JSON.stringify(updatePayload),
      });
      state.barcodeSettings = settingsResp.settings || payload;
      state.appUpdateSettings = updateResp.settings || { ...state.appUpdateSettings, ...updatePayload };
      state.readerInfo = { ...(state.readerInfo || {}), app_version: updatePayload.current_version || state.readerInfo?.app_version };
      syncAppVersionUI();
      refreshAppUpdateStatus(true).catch(() => {});
      await loadBarcodeSettingsView();
      showToast("Setari salvate", "success");
    });
  }
  if (testBtn) {
    testBtn.addEventListener("click", async () => {
      await api("/api/barcode/test-print", { method: "POST", body: "{}" });
      showToast("Test print trimis", "success");
    });
  }
  if (refreshBtn) {
    refreshBtn.addEventListener("click", () => {
      loadBarcodeSettingsView().catch((e) => showToast(e.message || "Refresh failed", "error"));
    });
  }
  bindRerender();
}

function buildBarcodeSettingsState(current = {}) {
  const values = {};
  BARCODE_FIELD_SECTIONS.forEach((section) => {
    section.fields.forEach((field) => {
      values[field.key] = current[field.key] ?? field.default ?? "";
    });
  });
  Object.entries(current || {}).forEach(([key, value]) => {
    if (key === "auth_username" || key === "auth_password" || key === "log_db_path") return;
    if (!(key in values)) values[key] = value ?? "";
  });
  return values;
}

function renderBarcodeField(field, values, printers) {
  const current = String(values[field.key] ?? field.default ?? "");
  const showFor = field.showFor ? field.showFor.join(",") : "";
  const style = field.span === 2 ? "grid-column:span 2;" : field.span === 3 ? "grid-column:span 3;" : "";
  let control = "";
  if (field.type === "printer-select") {
    const listID = `printer-options-${escapeHtml(field.key)}`;
    const options = printers.map((name) => `<option value="${escapeHtml(name)}"></option>`).join("");
    control = [
      `<input data-setting-key="${escapeHtml(field.key)}" value="${escapeHtml(current)}" list="${listID}" placeholder="\\\\server\\printer sau nume local">`,
      `<datalist id="${listID}">${options}</datalist>`,
      `<small>Puteti selecta din lista sau introduce manual numele exact al imprimantei shared.</small>`,
    ].join("");
  } else if (field.type === "select") {
    control = `<select data-setting-key="${escapeHtml(field.key)}">${(field.options || []).map(([value, label]) => `<option value="${escapeHtml(value)}" ${value === current ? "selected" : ""}>${escapeHtml(label)}</option>`).join("")}</select>`;
  } else {
    const inputMode = field.type === "number" ? `inputmode="decimal"` : "";
    control = `<input ${inputMode} data-setting-key="${escapeHtml(field.key)}" value="${escapeHtml(current)}">`;
  }
  return `<label class="barcode-field" data-key="${escapeHtml(field.key)}" data-show-for="${escapeHtml(showFor)}" style="${style}"><span>${escapeHtml(field.label)}</span>${control}</label>`;
}

function renderBarcodeUpdateField(label, key, current, type = "text", options = null, span = 1) {
  const style = span === 2 ? "grid-column:span 2;" : "";
  let control = "";
  if (type === "select" && Array.isArray(options)) {
    control = `<select data-update-setting-key="${escapeHtml(key)}">${options.map(([value, text]) => `<option value="${escapeHtml(value)}" ${String(current) === String(value) ? "selected" : ""}>${escapeHtml(text)}</option>`).join("")}</select>`;
  } else {
    control = `<input type="${escapeHtml(type)}" data-update-setting-key="${escapeHtml(key)}" value="${escapeHtml(current || "")}">`;
  }
  return `<label class="barcode-field" style="${style}"><span>${escapeHtml(label)}</span>${control}</label>`;
}

function applyBarcodeFieldVisibility() {
  const type = String(els.settingsPanelAnalytes.querySelector('[data-setting-key="othercfg_printer_barcode"]')?.value || "B3");
  els.settingsPanelAnalytes.querySelectorAll(".barcode-field").forEach((node) => {
    const showFor = String(node.dataset.showFor || "").trim();
    node.hidden = !!showFor && !showFor.split(",").includes(type);
  });
}

function renderBarcodeSettingsPreview(settings) {
  const host = document.getElementById("barcode-preview");
  if (!host) return;
  const dpi = Math.max(200, parseInt(settings.othercfg_printer_resolution || "200", 10) || 200);
  const labelWidthMM = Math.max(20, parseFloat(settings.othercfg_label_width || 35) || 35);
  const labelHeightMM = Math.max(15, parseFloat(settings.othercfg_label_height || 25) || 25);
  const labelWidthDots = Math.max(120, mmToDots(labelWidthMM, dpi));
  const labelHeightDots = Math.max(80, mmToDots(labelHeightMM, dpi));
  const maxWidth = 380;
  const scale = Math.min(maxWidth / labelWidthDots, 2.25);
  const widthPx = Math.round(labelWidthDots * scale);
  const heightPx = Math.round(labelHeightDots * scale);
  const originX = dotsToPreviewPx(mmToDots(settings.othercfg_label_ox || 0, dpi), scale);
  const originY = dotsToPreviewPx(mmToDots(settings.othercfg_label_oy || 0, dpi), scale);
  const barcodeX = originX + dotsToPreviewPx(mmToDots(settings.othercfg_print_bcodex || 0, dpi), scale);
  const barcodeY = originY + dotsToPreviewPx(mmToDots(settings.othercfg_print_bcodey || 0, dpi), scale);
  const barcodeTextX = originX + dotsToPreviewPx(mmToDots(settings.othercfg_print_bcodetxtx || 0, dpi), scale);
  const barcodeTextY = originY + dotsToPreviewPx(mmToDots(settings.othercfg_print_bcodetxty || 0, dpi), scale);
  const patientX = originX + dotsToPreviewPx(mmToDots(settings.othercfg_print_namex || 0, dpi), scale);
  const patientY = originY + dotsToPreviewPx(mmToDots(settings.othercfg_print_namey || 0, dpi), scale);
  const tubeX = originX + dotsToPreviewPx(mmToDots(settings.othercfg_print_tubecodex || 0, dpi), scale);
  const tubeY = originY + dotsToPreviewPx(mmToDots(settings.othercfg_print_tubecodey || 0, dpi), scale);
  const tubeW = Math.max(12, dotsToPreviewPx(mmToDots(settings.othercfg_print_tubecode_boxw || 0, dpi), scale));
  const tubeH = Math.max(12, dotsToPreviewPx(mmToDots(settings.othercfg_print_tubecode_boxh || 0, dpi), scale));
  const barcodeType = String(settings.othercfg_printer_barcode || "B3");
  const barcodeValue = "240015990123";
  const patientName = "Popescu Ion";
  const tubeCode = "A";
  const barcodeGraphic = isMatrixBarcode(barcodeType)
    ? renderMatrixBarcodePreview(barcodeType, barcodeX, barcodeY, barcodeValue, scale, settings, dpi)
    : renderLinearBarcodePreview(barcodeType, barcodeX, barcodeY, barcodeValue, scale, settings, dpi);
  const patientFont = previewFontSpec(settings.othercfg_print_namef, settings.othercfg_print_namew, settings.othercfg_print_nameh, scale);
  const barcodeTextFont = previewFontSpec(settings.othercfg_print_bcodetxtf, settings.othercfg_print_bcodetxtw, settings.othercfg_print_bcodetxth, scale);
  const tubeFont = previewFontSpec(settings.othercfg_print_tubecodef, settings.othercfg_print_tubecodew, settings.othercfg_print_tubecodeh, scale);
  host.innerHTML = `
    <div class="barcode-preview-meta">
      <span>${escapeHtml((BARCODE_TYPE_OPTIONS.find(([value]) => value === barcodeType) || [barcodeType, barcodeType])[1])}</span>
      <span>${escapeHtml(`${labelWidthMM}mm x ${labelHeightMM}mm`)}</span>
    </div>
    <svg viewBox="0 0 ${widthPx} ${heightPx}" class="barcode-preview-svg" preserveAspectRatio="xMidYMid meet">
      <rect x="1" y="1" width="${Math.max(0, widthPx - 2)}" height="${Math.max(0, heightPx - 2)}" rx="12" ry="12" fill="#fffefb" stroke="#cad6ce" stroke-width="2"></rect>
      ${barcodeGraphic}
      ${renderPreviewText(patientName, patientX, patientY, settings.othercfg_print_nameo, patientFont)}
      ${renderPreviewText(barcodeValue, barcodeTextX, barcodeTextY, settings.othercfg_print_bcodetxto, barcodeTextFont)}
      <rect x="${tubeX}" y="${tubeY}" width="${tubeW}" height="${tubeH}" rx="${Math.min(8, parseInt(settings.othercfg_print_tubecode_boxr || 4, 10) || 4)}" ry="${Math.min(8, parseInt(settings.othercfg_print_tubecode_boxr || 4, 10) || 4)}" fill="${String(settings.othercfg_print_tubecode_boxc || "B") === "W" ? "#fff" : "#111"}" stroke="#111" stroke-width="${Math.max(1, parseInt(settings.othercfg_print_tubecode_boxt || 1, 10) || 1)}"></rect>
      ${renderPreviewText(tubeCode, tubeX + 8, tubeY + tubeH - 6, settings.othercfg_print_tubecodeo, tubeFont, String(settings.othercfg_print_tubecode_boxc || "B") === "W" ? "#111" : "#fff")}
    </svg>
  `;
}

function mmToDots(value, dpi) {
  const mm = parseFloat(value || 0) || 0;
  return Math.round(mm * dpi / 25.4);
}

function isMatrixBarcode(type) {
  return ["B0", "BQ", "BX", "BD", "B7", "BF", "BB"].includes(String(type || ""));
}

function dotsToPreviewPx(value, scale) {
  return Math.round((Number(value || 0) || 0) * scale);
}

function renderLinearBarcodePreview(type, x, y, value, scale, settings, dpi) {
  const encoded = encodeLinearBarcode(type, value, settings);
  const narrowDot = Math.max(1, parseInt(settings.othercfg_print_bcodeopt_w || settings.othercfg_print_bcodeopt_nbw || "2", 10) || 2);
  const wideRatio = Math.max(2, parseFloat(settings.othercfg_bc_widenarowr || settings.othercfg_print_bcodeopt_ctnwn || "3.0") || 3);
  const modulePx = Math.max(1, dotsToPreviewPx(narrowDot, scale));
  const barHeight = Math.max(24, dotsToPreviewPx(parseInt(settings.othercfg_print_bcodeopt_h || "100", 10) || 100, scale));
  if (!encoded) {
    return renderPseudoLinearBarcode(x, y, value, modulePx, barHeight);
  }
  let cursor = x;
  const bars = [];
  for (const item of encoded) {
    const widthModules = item === "w" ? wideRatio : 1;
    const segmentPx = Math.max(1, Math.round(modulePx * widthModules));
    if (item !== "0" && item !== "n") {
      bars.push(`<rect x="${cursor}" y="${y}" width="${segmentPx}" height="${barHeight}" fill="#111"></rect>`);
    }
    cursor += segmentPx;
  }
  return `<g>${bars.join("")}</g>`;
}

function renderPseudoLinearBarcode(x, y, value, modulePx, barHeight) {
  const bars = [];
  let cursor = x;
  const source = `${value}WM`;
  for (let i = 0; i < source.length; i += 1) {
    const code = source.charCodeAt(i);
    for (let bit = 0; bit < 7; bit += 1) {
      const on = ((code >> bit) & 1) === 1;
      const w = on ? modulePx * 2 : modulePx;
      if (on) bars.push(`<rect x="${cursor}" y="${y}" width="${w}" height="${barHeight}" fill="#111"></rect>`);
      cursor += w + 1;
    }
    cursor += modulePx;
  }
  return `<g>${bars.join("")}</g>`;
}

function renderMatrixBarcodePreview(type, x, y, value, scale, settings) {
  const quality = Math.max(6, parseInt(settings.othercfg_print_bcodeopt_dmq || "0", 10) || 14);
  const size = Math.max(4, Math.round(scale * Math.max(0.8, quality / 120)));
  const cells = [];
  const seed = `${type}:${value}:PREVIEW`;
  for (let row = 0; row < 14; row += 1) {
    for (let col = 0; col < 14; col += 1) {
      const idx = (row * 14 + col) % seed.length;
      const on = ((seed.charCodeAt(idx) + row * 13 + col * 7) % 3) !== 0;
      if (on) cells.push(`<rect x="${x + col * size}" y="${y + row * size}" width="${size - 1}" height="${size - 1}" fill="#111"></rect>`);
    }
  }
  return `<g>${cells.join("")}</g>`;
}

function renderPreviewText(text, x, y, orientation, font, fill = "#111") {
  const safe = escapeHtml(text);
  const size = Math.max(6, Number(font.size || 12));
  const family = font.family || "Arial, sans-serif";
  const weight = font.weight || "500";
  const stretch = Math.max(0.4, Number(font.stretch || 1));
  const transformScale = stretch === 1 ? "" : ` scale(${stretch} 1)`;
  const baseAttrs = `fill="${fill}" font-size="${size}" font-family="${family}" font-weight="${weight}" textLength="${Math.max(size, size * Math.max(1, text.length * stretch * 0.6))}" lengthAdjust="spacingAndGlyphs"`;
  if (orientation === "R") {
    return `<text x="${x}" y="${y}" ${baseAttrs} transform="rotate(90 ${x} ${y})${transformScale}">${safe}</text>`;
  }
  if (orientation === "I") {
    return `<text x="${x}" y="${y}" ${baseAttrs} transform="rotate(180 ${x} ${y})${transformScale}">${safe}</text>`;
  }
  if (orientation === "B") {
    return `<text x="${x}" y="${y}" ${baseAttrs} transform="rotate(-90 ${x} ${y})${transformScale}">${safe}</text>`;
  }
  return `<text x="${x}" y="${y}" ${baseAttrs} transform="${transformScale.trim()}">${safe}</text>`;
}

function previewFontSpec(fontCode, widthDots, heightDots, scale) {
  const size = Math.max(7, dotsToPreviewPx(parseInt(heightDots || "18", 10) || 18, scale));
  const stretch = Math.max(0.55, (parseInt(widthDots || "12", 10) || 12) / Math.max(1, parseInt(heightDots || "18", 10) || 18));
  const code = String(fontCode || "A").toUpperCase();
  const map = {
    A: { family: "Arial, Helvetica, sans-serif", weight: "500" },
    B: { family: "\"Arial Narrow\", Arial, Helvetica, sans-serif", weight: "700" },
    C: { family: "\"Courier New\", monospace", weight: "700" },
    D: { family: "\"OCR-B\", \"Courier New\", monospace", weight: "500" },
    E: { family: "Georgia, serif", weight: "600" },
    F: { family: "\"Helvetica Neue\", Arial, sans-serif", weight: "400" },
    G: { family: "\"Trebuchet MS\", Arial, sans-serif", weight: "700" },
    H: { family: "\"Times New Roman\", serif", weight: "700" },
    "0": { family: "\"Courier New\", monospace", weight: "500" },
  };
  return { ...(map[code] || map.A), size, stretch };
}

function encodeLinearBarcode(type, value, settings) {
  switch (String(type || "")) {
    case "BE":
      return encodeEAN13(value);
    case "B8":
      return encodeEAN8(value);
    case "BU":
      return encodeUPCA(value);
    case "B3":
    case "BL":
    case "BT":
      return encodeCode39(value, settings);
    case "BK":
      return encodeCodabar(value, settings);
    default:
      return null;
  }
}

function encodeEAN13(value) {
  const digits = String(value || "").replace(/\D/g, "");
  const normalized = digits.length >= 13 ? digits.slice(0, 13) : digits.length === 12 ? `${digits}${eanChecksum(digits)}` : "2400159901238";
  const parity = ["LLLLLL","LLGLGG","LLGGLG","LLGGGL","LGLLGG","LGGLLG","LGGGLL","LGLGLG","LGLGGL","LGGLGL"][Number(normalized[0])];
  const L = ["0001101","0011001","0010011","0111101","0100011","0110001","0101111","0111011","0110111","0001011"];
  const G = ["0100111","0110011","0011011","0100001","0011101","0111001","0000101","0010001","0001001","0010111"];
  const R = ["1110010","1100110","1101100","1000010","1011100","1001110","1010000","1000100","1001000","1110100"];
  let pattern = "101";
  for (let i = 1; i <= 6; i += 1) {
    const digit = Number(normalized[i]);
    pattern += parity[i - 1] === "L" ? L[digit] : G[digit];
  }
  pattern += "01010";
  for (let i = 7; i < 13; i += 1) pattern += R[Number(normalized[i])];
  pattern += "101";
  return pattern.split("");
}

function encodeEAN8(value) {
  const digits = String(value || "").replace(/\D/g, "");
  const normalized = digits.length >= 8 ? digits.slice(0, 8) : digits.length === 7 ? `${digits}${eanChecksum(`00000${digits}`)}`.slice(-8) : "12345670";
  const L = ["0001101","0011001","0010011","0111101","0100011","0110001","0101111","0111011","0110111","0001011"];
  const R = ["1110010","1100110","1101100","1000010","1011100","1001110","1010000","1000100","1001000","1110100"];
  let pattern = "101";
  for (let i = 0; i < 4; i += 1) pattern += L[Number(normalized[i])];
  pattern += "01010";
  for (let i = 4; i < 8; i += 1) pattern += R[Number(normalized[i])];
  pattern += "101";
  return pattern.split("");
}

function encodeUPCA(value) {
  const digits = String(value || "").replace(/\D/g, "");
  const normalized = digits.length >= 12 ? digits.slice(0, 12) : digits.length === 11 ? `${digits}${eanChecksum(`0${digits}`)}`.slice(-12) : "240015990123";
  return encodeEAN13(`0${normalized}`);
}

function eanChecksum(digits) {
  const arr = String(digits || "").replace(/\D/g, "").split("").map(Number);
  let sum = 0;
  for (let i = 0; i < arr.length; i += 1) {
    const fromRight = arr.length - i;
    sum += arr[i] * (fromRight % 2 === 0 ? 3 : 1);
  }
  return String((10 - (sum % 10)) % 10);
}

function encodeCode39(value) {
  const map = {
    "0": "nnnwwnwnn","1": "wnnwnnnnw","2": "nnwwnnnnw","3": "wnwwnnnnn","4": "nnnwwnnnw","5": "wnnwwnnnn","6": "nnwwwnnnn","7": "nnnwnnwnw","8": "wnnwnnwnn","9": "nnwwnnwnn",
    A: "wnnnnwnnw", B: "nnwnnwnnw", C: "wnwnnwnnn", D: "nnnnwwnnw", E: "wnnnwwnnn", F: "nnwnwwnnn", G: "nnnnnwwnw", H: "wnnnnwwnn", I: "nnwnnwwnn", J: "nnnnwwwnn",
    K: "wnnnnnnww", L: "nnwnnnnww", M: "wnwnnnnwn", N: "nnnnwnnww", O: "wnnnwnnwn", P: "nnwnwnnwn", Q: "nnnnnnwww", R: "wnnnnnwwn", S: "nnwnnnwwn", T: "nnnnwnwwn",
    U: "wwnnnnnnw", V: "nwwnnnnnw", W: "wwwnnnnnn", X: "nwnnwnnnw", Y: "wwnnwnnnn", Z: "nwwnwnnnn", "-": "nwnnnnwnw", ".": "wwnnnnwnn", " ": "nwwnnnwnn", "$": "nwnwnwnnn",
    "/": "nwnwnnnwn", "+": "nwnnnwnwn", "%": "nnnwnwnwn", "*": "nwnnwnwnn",
  };
  const data = `*${String(value || "").toUpperCase()}*`;
  const pattern = [];
  for (let i = 0; i < data.length; i += 1) {
    const token = map[data[i]] || map["*"];
    for (let j = 0; j < token.length; j += 1) {
      const char = token[j];
      const isBar = j % 2 === 0;
      pattern.push(isBar ? char : "0");
    }
    if (i < data.length - 1) pattern.push("0");
  }
  return pattern;
}

function encodeCodabar(value) {
  const map = {
    "0":"nnnnnww","1":"nnnnwwn","2":"nnnwnnw","3":"wwnnnnn","4":"nnwnnwn","5":"wnnnnwn","6":"nwnnnwn","7":"nwnnwnn","8":"nwwnnnn","9":"wnnwnnn",
    "-":"nnnwwnn","$":"nnwwnnn",":":"wnnnwnw","/":"wnwnnnw",".":"wnwnwnn","+":"nnwnwnw","A":"nnwwnwn","B":"nwnwnnw","C":"nnnwnww","D":"nnnwwwn",
  };
  const data = (() => {
    const raw = String(value || "").toUpperCase();
    if (/^[ABCD].*[ABCD]$/.test(raw)) return raw;
    return `A${raw}B`;
  })();
  const pattern = [];
  for (let i = 0; i < data.length; i += 1) {
    const token = map[data[i]] || map["A"];
    for (let j = 0; j < token.length; j += 1) {
      const char = token[j];
      const isBar = j % 2 === 0;
      pattern.push(isBar ? char : "0");
    }
    if (i < data.length - 1) pattern.push("0");
  }
  return pattern;
}

async function loadBarcodeHistory() {
  if (!state.barcodeMode) return;
  const dateFrom = encodeURIComponent(offsetISODate(-30));
  const dateTo = encodeURIComponent(localISODate());
  const resp = await api(`/api/barcode/jobs?limit=300&date_from=${dateFrom}&date_to=${dateTo}`);
  const rows = resp.jobs || [];
  els.ordersLayout.innerHTML = `
    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th>Data</th>
            <th>IP</th>
            <th>FileID</th>
            <th>Nume</th>
            <th>Tip BC</th>
            <th>Etichete</th>
            <th>Status</th>
            <th>Eroare</th>
          </tr>
        </thead>
        <tbody>
          ${rows.map((item) => `
            <tr>
              <td>${escapeHtml(formatDate(item.created_at))}</td>
              <td>${escapeHtml(item.client_ip || "-")}</td>
              <td>${escapeHtml(item.file_id || "-")}</td>
              <td>${escapeHtml(item.name || "-")}</td>
              <td>${escapeHtml(item.bc_type || "-")}</td>
              <td>${escapeHtml(String(item.labels_count ?? 0))}</td>
              <td>${escapeHtml(item.status || "-")}</td>
              <td>${escapeHtml(item.error || "-")}</td>
            </tr>
          `).join("")}
        </tbody>
      </table>
    </div>
  `;
  els.orderDetails.innerHTML = `<div class="muted">${rows.length} inregistrari</div>`;
}

async function loadLogs() {
  if (els.logsPanel.hidden) return;
  const resp = await api("/api/logs?limit=40");
  els.logsList.innerHTML = (resp.logs || []).map((item) => {
    const payload = formatLogPayload(item.payload);
    return `<div class="log-item"><div class="meta"><span>${escapeHtml(item.level.toUpperCase())} · ${escapeHtml(item.event_type)}</span><span>${escapeHtml(formatDate(item.created_at))}</span></div><div>${escapeHtml(item.message)}</div>${payload ? `<pre class="log-payload">${escapeHtml(payload)}</pre>` : ""}</div>`;
  }).join("") || `<div class="log-item">${escapeHtml(t("noLogs"))}</div>`;
}

function onLogPollChange() {
  const value = Number(els.logPollSeconds.value || 0);
  state.logPollSeconds = Number.isFinite(value) && value >= 0 ? value : 1;
  restartLogPolling();
}

function restartLogPolling() {
  if (state.logPollTimer) {
    clearInterval(state.logPollTimer);
    state.logPollTimer = null;
  }
  if (state.logPollSeconds > 0 && !els.logsPanel.hidden) {
    state.logPollTimer = setInterval(() => {
      if (!els.dashboardView.hidden) {
        loadLogs().catch(() => {});
      }
    }, state.logPollSeconds * 1000);
  }
}

function restartStatusPolling() {
  if (state.statusPollTimer) {
    clearInterval(state.statusPollTimer);
    state.statusPollTimer = null;
  }
  if (state.statusPollSeconds > 0) {
    state.statusPollTimer = setInterval(() => {
      if (!els.dashboardView.hidden) {
        loadStatus().catch(() => {});
        loadDashboard().catch(() => {});
      }
    }, state.statusPollSeconds * 1000);
  }
}

async function loadAnalytes() {
  const resp = await api("/api/analytes");
  state.analytes = resp.analytes || [];
  if (state.selectedTag && !state.analytes.some((item) => item.tag === state.selectedTag)) {
    state.selectedTag = null;
  }
  if (!state.selectedTag && state.analytes.length > 0) {
    state.selectedTag = state.analytes[0].tag;
  }
  if (state.selectedQCTargetAnalyteTag && !state.analytes.some((item) => item.tag === state.selectedQCTargetAnalyteTag)) {
    state.selectedQCTargetAnalyteTag = null;
  }
  renderAnalyteList();
  syncQCTargetAnalyteOptions();
  syncQCTargetFormAnalyteOptions();
  syncDailyDetailAnalyteOptions();
  if (state.currentView === "daily-details" || state.settingsSubView === "daily-details") {
    renderDailyDetailValuesWorkspace();
  }
  renderQCAnalyteFilter();
  renderQCTargetList();
}

async function loadDailyDetailDefinitions() {
  const resp = await api("/api/daily-details/definitions");
  state.dailyDetailDefinitions = resp.definitions || [];
  renderDailyDetailDefinitionList();
}

async function loadDailyDetailValues(scopeDate, roundNo) {
  const params = new URLSearchParams();
  params.set("scope_date", scopeDate || localISODate());
  if (roundNo > 0) params.set("round_no", String(roundNo));
  const resp = await api(`/api/daily-details?${params.toString()}`);
  state.dailyDetailValues = resp.values || [];
}

async function loadDailyDetailsWorkspace() {
  state.dailyDetailsDate = state.dailyDetailsDate || localISODate();
  if (els.dailyDetailsDate) {
    els.dailyDetailsDate.value = state.dailyDetailsDate;
  }
  await loadDailyDetailRounds();
  syncDailyDetailAnalyteOptions();
  const usesRound = ["day_round", "day_round_analyte"].includes(state.dailyDetailsScopeTab);
  await loadDailyDetailValues(state.dailyDetailsDate, usesRound ? Number(state.dailyDetailsRoundNo || 0) : 0);
  renderDailyDetailValuesWorkspace();
}

async function loadDailyDetailRounds() {
  const params = new URLSearchParams();
  params.set("order_date", state.dailyDetailsDate || localISODate());
  const resp = await api(`/api/orders/rounds?${params.toString()}`);
  state.dailyDetailsRounds = resp.rounds || [];
  if (state.dailyDetailsRounds.length === 0) {
    state.dailyDetailsRounds = [1];
  }
  if (!state.dailyDetailsRounds.includes(state.dailyDetailsRoundNo)) {
    state.dailyDetailsRoundNo = state.dailyDetailsRounds[0] || 1;
  }
  if (els.dailyDetailsRound) {
    els.dailyDetailsRound.innerHTML = state.dailyDetailsRounds.map((item) => `<option value="${item}">${item}</option>`).join("");
    els.dailyDetailsRound.value = String(state.dailyDetailsRoundNo || 1);
  }
}

function syncDailyDetailAnalyteOptions() {
  const items = knownAnalyteOptions();
  if (!state.dailyDetailsAnalyteTag || !items.some((item) => item.tag === state.dailyDetailsAnalyteTag)) {
    state.dailyDetailsAnalyteTag = items[0]?.tag || "";
  }
  if (els.dailyDetailsAnalyte) {
    els.dailyDetailsAnalyte.innerHTML = items.map((item) => `<option value="${escapeHtml(item.tag)}">${escapeHtml(item.tag)} · ${escapeHtml(item.name || "-")}</option>`).join("");
    els.dailyDetailsAnalyte.value = state.dailyDetailsAnalyteTag || "";
  }
}

function setAnalyteRefreshButtonState(loading) {
  els.refreshAnalytesBtn.disabled = loading;
  els.refreshAnalytesBtn.classList.toggle("is-loading", loading);
  els.refreshAnalytesBtn.textContent = t("refresh");
}

function setAnalyteSaveState(loading) {
  const button = document.getElementById("save-analyte");
  button.disabled = loading;
  button.classList.toggle("is-loading", loading);
}

async function onRefreshAnalytesClick() {
  setAnalyteRefreshButtonState(true);
  try {
    await Promise.all([loadAnalytes(), loadQCTargets()]);
    els.analyteMessage.textContent = "";
  } catch (error) {
    showToast(error?.message || "Cannot load analytes", "error");
  } finally {
    setAnalyteRefreshButtonState(false);
  }
}

async function loadOrders() {
  const requestedRoundNo = state.selectedRoundNo;
  const params = new URLSearchParams();
  params.set("include_analysis", "1");
  if (requestedRoundNo > 0) params.set("round_no", String(requestedRoundNo));
  if (state.selectedOrderDate) params.set("order_date", state.selectedOrderDate);
  const query = params.toString() ? `?${params.toString()}` : "";
  const resp = await api(`/api/orders${query}`);
  state.orders = resp.orders || [];
  state.rounds = resp.rounds || [];
  state.selectedOrderDate = resp.order_date || state.selectedOrderDate;
  if (state.rounds.length === 0) {
    state.rounds = [1];
  }
  const responseRoundNo = Number(resp.round_no || 0);
  const selectedRoundExists = state.rounds.includes(state.selectedRoundNo);
  if (responseRoundNo > 0) {
    state.selectedRoundNo = responseRoundNo;
  } else if (!selectedRoundExists) {
    state.selectedRoundNo = state.rounds[state.rounds.length - 1] || 1;
  }
  syncOrderControls();
  renderRoundSelect();
  if (!state.selectedOrderId && state.orders.length > 0) {
    state.selectedOrderId = state.orders[0].order.id;
  }
  if (state.selectedOrderId && !state.orders.some((item) => item.order.id === state.selectedOrderId)) {
    state.selectedOrderId = state.orders[0]?.order?.id || null;
    state.selectedOrderAnalysisID = null;
  }
  state.selectedOrderIDs = state.selectedOrderIDs.filter((id) => state.orders.some((item) => item.order.id === id));
  renderOrdersLayout();
  renderOrderDetails();
}

function syncOrderControls() {
  if (state.barcodeMode) {
    els.importOrdersBtn.hidden = true;
    els.exportOrdersBtn.hidden = true;
    els.worklistOrdersBtn.hidden = true;
    els.newRoundBtn.hidden = true;
    els.ordersSelectAllBox.hidden = true;
    if (els.orderDate?.parentElement) els.orderDate.parentElement.hidden = true;
    if (els.roundSelect?.parentElement) els.roundSelect.parentElement.hidden = true;
    return;
  }
  if (els.orderDate) {
    els.orderDate.value = state.selectedOrderDate || "";
  }
  const fileMode = state.commType === "file";
  const caryMode = String(state.readerInfo?.protocol || state.readerInfo?.analyzer_code || "").toLowerCase() === "cary60-uvvis";
  els.importOrdersBtn.hidden = !fileMode;
  els.exportOrdersBtn.hidden = !fileMode;
  els.worklistOrdersBtn.hidden = !(fileMode && caryMode);
  els.newRoundBtn.hidden = !fileMode;
  els.ordersSelectAllBox.hidden = !fileMode;
}

function onRefreshOrdersClick() {
  if (state.barcodeMode) {
    loadBarcodeHistory().catch((error) => showToast(error?.message || "Cannot load history", "error"));
    return;
  }
  loadOrders().catch((error) => showToast(error?.message || "Cannot load orders", "error"));
}

function syncQCControls() {
  if (els.qcPeriod && !els.qcPeriod.options.length) {
    els.qcPeriod.innerHTML = [
      ["current_week", t("westgardCurrentWeek")],
      ["previous_week", t("westgardPreviousWeek")],
      ["current_month", t("westgardCurrentMonth")],
      ["previous_month", t("westgardPreviousMonth")],
      ["current_year", t("westgardCurrentYear")],
      ["custom", t("westgardCustom")],
    ].map(([value, label]) => `<option value="${escapeHtml(value)}">${escapeHtml(label)}</option>`).join("");
  }
  if (els.qcPeriod) {
    els.qcPeriod.value = state.selectedQCPeriod || "current_month";
  }
  if (els.qcDateFrom) {
    els.qcDateFrom.value = state.selectedQCDateFrom || "";
    els.qcDateFrom.disabled = (state.selectedQCPeriod || "current_month") !== "custom";
  }
  if (els.qcDateTo) {
    els.qcDateTo.value = state.selectedQCDateTo || "";
    els.qcDateTo.disabled = (state.selectedQCPeriod || "current_month") !== "custom";
  }
  if (els.qcAnalyteFilter) {
    els.qcAnalyteFilter.value = state.selectedQCAnalyteFilter || "";
  }
  if (els.qcLevelFilter) {
    els.qcLevelFilter.value = state.selectedQCLevel || "";
  }
}

function onOrderDateChange() {
  state.selectedOrderDate = els.orderDate.value || "";
  state.selectedRoundNo = 1;
  state.rounds = [1];
  state.selectedOrderId = null;
  state.selectedOrderAnalysisID = null;
  state.selectedOrderIDs = [];
  syncOrderControls();
  renderRoundSelect();
  loadOrdersWithFeedback();
}

function onRoundChange() {
  state.selectedRoundNo = Number(els.roundSelect.value || 0);
  state.selectedOrderId = null;
  state.selectedOrderAnalysisID = null;
  loadOrdersWithFeedback();
}

function onQCPeriodChange() {
  state.selectedQCPeriod = els.qcPeriod?.value || "current_month";
  if (state.selectedQCPeriod !== "custom") {
    const bounds = currentPeriodBounds(state.selectedQCPeriod);
    state.selectedQCDateFrom = bounds.from;
    state.selectedQCDateTo = bounds.to;
  }
  state.selectedQCDate = state.selectedQCDateFrom || "";
  state.selectedQCRecordId = null;
  state.selectedQCAnalysisId = null;
  syncQCControls();
  loadQCRecords().catch(() => {});
}

function onQCDateRangeChange() {
  state.selectedQCPeriod = "custom";
  state.selectedQCDateFrom = els.qcDateFrom?.value || "";
  state.selectedQCDateTo = els.qcDateTo?.value || "";
  state.selectedQCDate = state.selectedQCDateFrom || "";
  state.selectedQCRecordId = null;
  state.selectedQCAnalysisId = null;
  syncQCControls();
  loadQCRecords().catch(() => {});
}

function onQCRoundChange() {
  state.selectedQCRoundNo = Number(els.qcRoundSelect.value || 0);
}

function onQCLevelFilterChange() {
  state.selectedQCLevel = els.qcLevelFilter.value || "";
  syncQCSelectionAfterFilter();
}

function onQCAnalyteFilterChange() {
  state.selectedQCAnalyteFilter = els.qcAnalyteFilter.value || "";
  syncQCSelectionAfterFilter();
}

function syncQCSelectionAfterFilter() {
  const filtered = filteredQCRows();
  if (!filtered.some((item) => item.record.id === state.selectedQCRecordId && item.analysis.id === state.selectedQCAnalysisId)) {
    state.selectedQCRecordId = filtered[0]?.record?.id || null;
    state.selectedQCAnalysisId = filtered[0]?.analysis?.id || null;
    state.selectedQCAnalysisTag = filtered[0]?.analysis?.analyte_tag || null;
  }
  renderQCLayout();
  renderQCDetails();
}

function onQCWestgardPeriodChange() {
  state.qcWestgardPeriod = els.qcWestgardPeriod.value || "current_month";
  syncQCWestgardPeriodControls();
  if (!els.qcWestgardModal.hidden) {
    onGenerateQCWestgard().catch((error) => showToast(error.message, "error"));
  }
}

function onQCWestgardCustomDateChange() {
  if ((els.qcWestgardDateFrom.value || "") && (els.qcWestgardDateTo.value || "")) {
    state.qcWestgardPeriod = "custom";
    els.qcWestgardPeriod.value = "custom";
    syncQCWestgardPeriodControls();
    if (!els.qcWestgardModal.hidden) {
      onGenerateQCWestgard().catch((error) => showToast(error.message, "error"));
    }
  }
}

function onShowQCWestgardIssues() {
  const issues = Array.isArray(state.qcWestgardMetrics?.validation_issues) ? state.qcWestgardMetrics.validation_issues : [];
  if (!issues.length) {
    window.alert(t("westgardValid"));
    return;
  }
  window.alert(issues.join("\n"));
}

function onDailyDetailsDateChange() {
  state.dailyDetailsDate = els.dailyDetailsDate.value || localISODate();
  loadDailyDetailsWorkspace().catch((error) => showToast(error?.message || "Cannot load daily details", "error"));
}

function onDailyDetailsRoundChange() {
  state.dailyDetailsRoundNo = Number(els.dailyDetailsRound.value || 0);
  loadDailyDetailsWorkspace().catch((error) => showToast(error?.message || "Cannot load daily details", "error"));
}

function onDailyDetailsAnalyteChange() {
  state.dailyDetailsAnalyteTag = els.dailyDetailsAnalyte.value || "";
  renderDailyDetailValuesWorkspace();
}

function onDailyDetailsScopeChange(scope) {
  state.dailyDetailsScopeTab = scope;
  syncDailyDetailAnalyteOptions();
  renderDailyDetailValuesWorkspace();
  loadDailyDetailsWorkspace().catch((error) => showToast(error?.message || "Cannot load daily details", "error"));
}

function onOrdersSelectAllChange() {
  if (els.ordersSelectAll.checked) {
    state.selectedOrderIDs = state.orders.map((item) => item.order.id);
  } else {
    state.selectedOrderIDs = [];
  }
  renderOrdersLayout();
}

function onImportOrdersClick() {
  els.ordersImportFile.value = "";
  els.ordersImportFile.click();
}

function setImportButtonState(loading) {
  els.importOrdersBtn.disabled = loading;
  els.importOrdersBtn.classList.toggle("is-loading", loading);
  els.importOrdersBtn.textContent = loading ? t("importing") : t("importFile");
}

function hideToast() {
  if (state.toastTimer) {
    clearTimeout(state.toastTimer);
    state.toastTimer = null;
  }
  els.appToast.hidden = true;
  els.appToast.classList.remove("success", "error");
  els.appToastBody.textContent = "";
}

function showToast(message, kind = "success") {
  hideToast();
  els.appToastBody.textContent = message;
  els.appToast.hidden = false;
  els.appToast.classList.add(kind === "error" ? "error" : "success");
  state.toastTimer = setTimeout(hideToast, kind === "error" ? 10000 : 1000);
}

function buildImportMessage(payload, fileName) {
  const imported = Number(payload?.imported || 0);
  const warnings = Number(payload?.warnings || 0);
  let message = `${t("importSuccess")}: ${payload?.file_name || fileName}`;
  if (imported > 0) {
    message += ` • ${imported} ${t("rowsImported")}`;
  }
  if (warnings > 0) {
    message += ` • ${warnings} ${t("warningCount")}`;
  }
  return message;
}

async function onOrdersImportFileChange() {
  const [file] = els.ordersImportFile.files || [];
  if (!file) return;
  setImportButtonState(true);
  try {
    const form = new FormData();
    form.append("file", file);
    form.append("order_date", state.selectedOrderDate || localISODate());
    const response = await fetch("/api/orders/import", {
      method: "POST",
      credentials: "same-origin",
      body: form,
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok || payload.ok === false) {
      throw new Error(payload.error || `Request failed with ${response.status}`);
    }
    showToast(buildImportMessage(payload, file.name), "success");
    await Promise.all([
      loadOrders(),
      loadDashboard(),
      loadStatus(),
      loadLogs().catch(() => {}),
    ]);
  } catch (error) {
    showToast(`${t("importFailed")}: ${error.message}`, "error");
    await loadLogs().catch(() => {});
  } finally {
    setImportButtonState(false);
    els.ordersImportFile.value = "";
  }
}

async function onExportOrdersClick() {
  try {
    const resp = await api("/api/orders/export", {
      method: "POST",
      body: JSON.stringify({
        order_ids: state.selectedOrderIDs,
        order_date: state.selectedOrderDate,
      }),
    });
    window.alert(`${t("exportedFile")}: ${resp.path}`);
  } catch (error) {
    window.alert(error.message);
  }
}

function onWorklistOrdersClick() {
  const params = new URLSearchParams();
  if (state.selectedOrderDate) params.set("order_date", state.selectedOrderDate);
  if (state.selectedRoundNo > 0) params.set("round_no", String(state.selectedRoundNo));
  window.open(`/api/orders/worklist?${params.toString()}`, "_blank", "noopener,noreferrer");
}

async function onNewRoundClick() {
  try {
    const resp = await api("/api/orders/rounds", {
      method: "POST",
      body: JSON.stringify({
        order_date: state.selectedOrderDate || localISODate(),
      }),
    });
    state.selectedOrderDate = resp.order_date || state.selectedOrderDate;
    state.rounds = resp.rounds || state.rounds;
    state.selectedRoundNo = Number(resp.round_no || 1);
    state.selectedOrderId = null;
    state.selectedOrderAnalysisID = null;
    state.selectedOrderIDs = [];
    await loadOrdersWithFeedback();
  } catch (error) {
    window.alert(error.message);
  }
}

async function loadOrdersWithFeedback() {
  try {
    await loadOrders();
  } catch (error) {
    syncOrderControls();
    renderRoundSelect();
    const message = error?.message || "Cannot load orders";
    els.ordersLayout.innerHTML = `<div class="log-item">${escapeHtml(message)}</div>`;
    els.orderDetails.innerHTML = `<div class="log-item">${escapeHtml(message)}</div>`;
  }
}

async function loadQCRecords() {
  if ((state.selectedQCPeriod || "current_month") !== "custom") {
    const bounds = currentPeriodBounds(state.selectedQCPeriod || "current_month");
    state.selectedQCDateFrom = bounds.from;
    state.selectedQCDateTo = bounds.to;
    state.selectedQCDate = bounds.from;
  }
  const params = new URLSearchParams();
  params.set("include_analysis", "1");
  if (state.selectedQCDateFrom) params.set("date_from", state.selectedQCDateFrom);
  if (state.selectedQCDateTo) params.set("date_to", state.selectedQCDateTo);
  if (state.selectedQCDate) params.set("run_date", state.selectedQCDate);
  const query = params.toString() ? `?${params.toString()}` : "";
  const resp = await api(`/api/qc-records${query}`);
  state.qcRecords = resp.qc_records || [];
  state.selectedQCDate = resp.run_date || state.selectedQCDate;
  state.selectedQCDateFrom = resp.date_from || state.selectedQCDateFrom;
  state.selectedQCDateTo = resp.date_to || state.selectedQCDateTo;
  syncQCControls();
  renderQCAnalyteFilter();
  const filtered = filteredQCRows();
  if (!state.selectedQCRecordId && filtered.length > 0) {
    state.selectedQCRecordId = filtered[0].record?.id || null;
    state.selectedQCAnalysisId = filtered[0].analysis?.id || null;
    state.selectedQCAnalysisTag = filtered[0].analysis?.analyte_tag || null;
  }
  if (state.selectedQCRecordId && !filtered.some((item) => item.record.id === state.selectedQCRecordId && item.analysis.id === state.selectedQCAnalysisId)) {
    state.selectedQCRecordId = filtered[0]?.record?.id || null;
    state.selectedQCAnalysisId = filtered[0]?.analysis?.id || null;
    state.selectedQCAnalysisTag = filtered[0]?.analysis?.analyte_tag || null;
  }
  state.qcWestgardMetrics = null;
  renderQCLayout();
  renderQCDetails();
}

async function loadQCMeta() {
  const resp = await api("/api/qc/meta");
  state.qcLevels = Array.isArray(resp.levels) ? resp.levels.map((item) => String(item || "").trim()).filter(Boolean) : [];
  syncQCConfiguredLevels();
}

async function loadQCTargets() {
  const resp = await api("/api/qc-targets");
  state.qcTargets = resp.qc_targets || [];
  if (state.selectedQCTargetID && !state.qcTargets.some((item) => item.id === state.selectedQCTargetID)) {
    state.selectedQCTargetID = null;
  }
  syncQCTargetAnalyteOptions();
  syncQCConfiguredLevels();
  syncQCTargetFormAnalyteOptions();
  syncQCRecordAnalyteOptions();
  syncDailyDetailAnalyteOptions();
  if (state.currentView === "daily-details" || state.settingsSubView === "daily-details") {
    renderDailyDetailValuesWorkspace();
  }
  renderQCAnalyteFilter();
  syncSelectedTargetFromForm();
  renderQCTargetList();
}

function configuredQCLevels(extra = []) {
  const values = [...state.qcLevels, ...extra.map((item) => String(item || "").trim()).filter(Boolean)];
  const seen = new Set();
  const out = [];
  values.forEach((value) => {
    if (!value || seen.has(value)) return;
    seen.add(value);
    out.push(value);
  });
  return out.length > 0 ? out : ["QC"];
}

function syncQCConfiguredLevels() {
  if (!els.qcTargetLevel) return;
  const levels = configuredQCLevels(state.qcTargets.map((item) => String(item.control_level || "").trim()));
  const current = String(els.qcTargetLevel.value || "").trim();
  els.qcTargetLevel.innerHTML = levels.map((level) => `<option value="${escapeHtml(level)}">${escapeHtml(level)}</option>`).join("");
  els.qcTargetLevel.value = levels.includes(current) ? current : (levels[0] || "");
}

function openQCRecordModal() {
  syncQCRecordAnalyteOptions();
  els.qcRecordDate.value = state.selectedQCDate || localISODate();
  syncQCRecordLotOptions();
  els.qcRecordLabel.dataset.autofill = "1";
  els.qcRecordQualitative.value = "";
  els.qcRecordQuantitative.value = "";
  els.qcRecordInterpretation.value = "";
  els.qcRecordMessage.textContent = "";
  els.qcRecordModal.hidden = false;
}

function closeQCRecordModal() {
  els.qcRecordModal.hidden = true;
  els.qcRecordMessage.textContent = "";
}

function syncQCRecordAnalyteOptions() {
  const activeItems = knownAnalyteOptions();
  const preferred = state.selectedQCAnalysisTag && activeItems.some((item) => item.tag === state.selectedQCAnalysisTag) ? state.selectedQCAnalysisTag : "";
  els.qcRecordAnalyte.innerHTML = [
    `<option value="">Selecteaza analiza</option>`,
    ...activeItems.map((item) => `<option value="${escapeHtml(item.tag)}">${escapeHtml(item.tag)} · ${escapeHtml(item.name || "-")}</option>`),
  ].join("");
  els.qcRecordAnalyte.value = preferred;
}

function syncQCRecordLotOptions() {
  const analyteTag = String(els.qcRecordAnalyte.value || "").trim();
  const targets = state.qcTargets.filter((item) => item.active !== false && item.analyte_tag === analyteTag);
  if (!analyteTag) {
    els.qcRecordLot.disabled = true;
    els.qcRecordLot.innerHTML = `<option value="">Selecteaza mai intai analiza</option>`;
    syncQCRecordLotMetadata();
    return;
  }
  els.qcRecordLot.disabled = targets.length === 0;
  els.qcRecordLot.innerHTML = targets.length > 0
    ? targets.map((item) => `<option value="${escapeHtml(String(item.id))}">${escapeHtml(item.lot_no || "-")} · ${escapeHtml(item.control_level || "-")}</option>`).join("")
    : `<option value="">Nu exista loturi QC definite pentru analiza selectata</option>`;
  syncQCRecordLotMetadata();
}

function syncQCRecordLotMetadata() {
  const targetID = Number(els.qcRecordLot.value || 0);
  const target = state.qcTargets.find((item) => item.id === targetID);
  const hasAnalyte = !!String(els.qcRecordAnalyte.value || "").trim();
  const level = target?.control_level || (hasAnalyte ? (state.selectedQCLevel || configuredQCLevels()[0] || "QC") : "");
  const lot = target?.lot_no || "";
  els.qcRecordLevel.value = level;
  if (!els.qcRecordLabel.value || els.qcRecordLabel.dataset.autofill === "1") {
    els.qcRecordLabel.value = `${level} ${lot}`.trim();
    els.qcRecordLabel.dataset.autofill = "1";
  }
}

async function onSaveQCRecord(event) {
  event.preventDefault();
  const analyteTag = els.qcRecordAnalyte.value || "";
  const analyte = state.analytes.find((item) => item.tag === analyteTag);
  if (!analyte) {
    els.qcRecordMessage.textContent = "Analiza este obligatorie";
    return;
  }
  const targetID = Number(els.qcRecordLot.value || 0);
  const target = state.qcTargets.find((item) => item.id === targetID);
  const quantitativeValue = (els.qcRecordQuantitative.value || "").trim();
  const payload = {
    run_date: els.qcRecordDate.value || localISODate(),
    analyte_tag: analyte.tag,
    analyte_name: analyte.name || analyte.tag,
    control_level: target?.control_level || els.qcRecordLevel.value || configuredQCLevels()[0] || "QC",
    lot_no: target?.lot_no || (els.qcRecordLabel.value || "-").trim(),
    control_label: (els.qcRecordLabel.value || `${target?.control_level || ""} ${target?.lot_no || "-"}`).trim(),
    result_value: quantitativeValue,
    raw_value: quantitativeValue,
    interpreted_value: "",
    unit: analyte.result_measure_unit || target?.unit || "",
  };
  try {
    await api("/api/qc-records", { method: "POST", body: JSON.stringify(payload) });
    closeQCRecordModal();
    state.selectedQCDate = payload.run_date;
    await Promise.all([loadQCRecords(), loadStatus(), loadDashboard()]);
    showToast(t("saveQcTarget"), "success");
  } catch (error) {
    els.qcRecordMessage.textContent = error.message;
  }
}

function renderAnalyteList() {
  const query = els.analyteSearch.value.trim().toLowerCase();
  const filtered = state.analytes.filter((item) => [item.tag, item.code, item.name, item.description].join(" ").toLowerCase().includes(query));
  if (filtered.length === 0) {
    els.analyteList.innerHTML = `<div class="log-item">${escapeHtml(t("noAnalytes"))}</div>`;
  } else {
    els.analyteList.innerHTML = `
      <div class="analyte-table-wrap">
        <table class="analyte-table">
          <thead>
            <tr>
              <th class="col-tag">Tag</th>
              <th class="col-code">Code</th>
              <th>Name</th>
              <th class="col-type">Type</th>
              <th class="col-state">State</th>
            </tr>
          </thead>
          <tbody>
            ${filtered.map((item) => `
              <tr class="${item.tag === state.selectedTag ? "active" : ""}" data-tag="${escapeHtml(item.tag)}">
                <td class="col-tag"><span class="cell-ellipsis" title="${escapeHtml(item.tag)}"><strong>${escapeHtml(item.tag)}</strong></span></td>
                <td class="col-code"><span class="cell-ellipsis" title="${escapeHtml(item.code || "-")}">${escapeHtml(item.code || "-")}</span></td>
                <td>
                  <div class="analyte-main">
                    <strong>${escapeHtml(item.name || "-")}</strong>
                    <div class="small muted">${escapeHtml(item.description || "")}</div>
                  </div>
                </td>
                <td class="col-type">${escapeHtml(item.result_type || "-")}</td>
                <td class="col-state"><span class="badge">${item.active ? "active" : "inactive"}</span></td>
              </tr>`).join("")}
          </tbody>
        </table>
      </div>`;
  }
  [...els.analyteList.querySelectorAll("[data-tag]")].forEach((button) => {
    button.addEventListener("click", () => {
      state.selectedTag = button.dataset.tag;
      renderAnalyteList();
      const item = state.analytes.find((entry) => entry.tag === state.selectedTag);
      if (item) {
        openAnalyteModal(item);
      }
    });
  });
}

function currentAnalyte() {
  return state.analytes.find((item) => item.tag === state.selectedTag) || null;
}

function selectedQCTarget() {
  const analyteTag = String(els.qcTargetAnalyte.value || "").trim();
  if (!analyteTag) return null;
  const controlLevel = String(els.qcTargetLevel.value || configuredQCLevels()[0] || "QC").trim();
  const lotNo = String(els.qcTargetLot.value || "-").trim() || "-";
  return state.qcTargets.find((item) => item.analyte_tag === analyteTag && item.control_level === controlLevel && (item.lot_no || "-") === lotNo) || null;
}

function currentQCTargetAnalyteTag() {
  return state.selectedQCTargetAnalyteTag || state.analytes[0]?.tag || "";
}

function syncQCTargetAnalyteOptions() {
  const activeItems = state.analytes.filter((item) => item.active !== false);
  if (state.selectedQCTargetAnalyteTag && !activeItems.some((item) => item.tag === state.selectedQCTargetAnalyteTag)) {
    state.selectedQCTargetAnalyteTag = null;
  }
  if (!state.selectedQCTargetAnalyteTag) {
    state.selectedQCTargetAnalyteTag = state.selectedTag || activeItems[0]?.tag || null;
  }
  els.qcTargetFilterAnalyte.innerHTML = activeItems.map((item) => `<option value="${escapeHtml(item.tag)}">${escapeHtml(item.tag)} · ${escapeHtml(item.name || "-")}</option>`).join("");
  if (state.selectedQCTargetAnalyteTag) {
    els.qcTargetFilterAnalyte.value = state.selectedQCTargetAnalyteTag;
  }
}

function syncQCTargetFormAnalyteOptions() {
  const activeItems = state.analytes.filter((item) => item.active !== false);
  els.qcTargetAnalyte.innerHTML = activeItems.map((item) => `<option value="${escapeHtml(item.tag)}">${escapeHtml(item.tag)} · ${escapeHtml(item.name || "-")}</option>`).join("");
  syncQCConfiguredLevels();
  if (currentQCTargetAnalyteTag()) {
    els.qcTargetAnalyte.value = currentQCTargetAnalyteTag();
  }
}

function onQCTargetFilterAnalyteChange() {
  state.selectedQCTargetAnalyteTag = els.qcTargetFilterAnalyte.value || null;
  state.selectedQCTargetID = null;
  renderQCTargetList();
}

function onQCTargetAnalyteChange() {
  syncSelectedTargetFromForm();
}

function syncSelectedTargetFromForm() {
  const analyteTag = String(els.qcTargetAnalyte.value || "").trim();
  const analyte = state.analytes.find((item) => item.tag === analyteTag) || null;
  if (!analyte) {
    if (!state.editingQCTargetID) {
      state.selectedQCTargetID = null;
    }
    els.qcTargetMessage.textContent = "";
    return;
  }
  const target = selectedQCTarget();
  if (!state.editingQCTargetID) {
    state.selectedQCTargetID = target?.id || null;
    els.qcTargetMean.value = target ? String(target.target_mean ?? "") : "";
    els.qcTargetSD.value = target ? String(target.target_sd ?? "") : "";
  }
  updateQCTargetDerivedFields();
  if (state.editingQCTargetID) {
    els.qcTargetMessage.textContent = `${t("editing")} ${analyte.tag}`;
  } else {
    els.qcTargetMessage.textContent = target ? `${t("editing")} ${analyte.tag} / ${target.control_level} / ${target.lot_no || "-"}` : t("readyNew");
  }
}

function updateQCTargetDerivedFields() {
  const mean = Number(els.qcTargetMean.value || 0);
  const sd = Number(els.qcTargetSD.value || 0);
  const cv = mean !== 0 ? Math.abs(sd / mean) * 100 : 0;
  els.qcTarget1SD.value = sd ? `± ${sd.toFixed(4)}` : "";
  els.qcTarget2SD.value = sd ? `± ${(sd * 2).toFixed(4)}` : "";
  els.qcTarget3SD.value = sd ? `± ${(sd * 3).toFixed(4)}` : "";
  els.qcTargetCV.value = mean && sd ? cv.toFixed(2) : "";
}

function renderQCTargetList() {
  const analyteTag = currentQCTargetAnalyteTag();
  const items = analyteTag ? state.qcTargets.filter((item) => item.analyte_tag === analyteTag) : [];
  if (state.selectedQCTargetID && !items.some((item) => item.id === state.selectedQCTargetID)) {
    state.selectedQCTargetID = null;
  }
  if (items.length === 0) {
    els.qcTargetList.innerHTML = `<div class="log-item">${escapeHtml(t("noQc"))}</div>`;
    return;
  }
  els.qcTargetList.innerHTML = `
    <div class="analyte-table-wrap">
      <table class="analyte-table">
        <thead>
          <tr>
            <th class="col-code">${escapeHtml(t("analysisTag"))}</th>
            <th>${escapeHtml(t("qcLevel"))}</th>
            <th>${escapeHtml(t("lotNo"))}</th>
            <th>${escapeHtml(t("qcTargetMean"))}</th>
            <th>${escapeHtml(t("qcTargetSD"))}</th>
            <th>${escapeHtml(t("qcTargetCV"))}</th>
          </tr>
        </thead>
        <tbody>
          ${items.map((item) => `<tr class="${item.id === state.selectedQCTargetID ? "active" : ""}" data-target-id="${item.id}">
            <td class="col-code"><span class="cell-ellipsis">${escapeHtml(item.analyte_tag || "-")}</span></td>
            <td>${escapeHtml(item.control_level || "-")}</td>
            <td>${escapeHtml(item.lot_no || "-")}</td>
            <td>${escapeHtml(String(item.target_mean ?? "-"))}</td>
            <td>${escapeHtml(String(item.target_sd ?? "-"))}</td>
            <td>${escapeHtml(String(item.target_cv ?? "-"))}</td>
          </tr>`).join("")}
        </tbody>
      </table>
    </div>`;
  [...els.qcTargetList.querySelectorAll("[data-target-id]")].forEach((row) => {
    row.addEventListener("click", async () => {
      const id = Number(row.dataset.targetId || 0);
      const target = state.qcTargets.find((item) => item.id === id);
      if (!target) return;
      if (!state.qcLevels.length) {
        await loadQCMeta().catch(() => {});
      }
      state.selectedQCTargetID = id;
      state.selectedQCTargetAnalyteTag = target.analyte_tag || currentQCTargetAnalyteTag();
      els.qcTargetFilterAnalyte.value = state.selectedQCTargetAnalyteTag;
      renderQCTargetList();
      openQCTargetModal(target);
    });
  });
}

function filteredDailyDetailDefinitions() {
  const query = String(els.dailyDetailDefinitionSearch?.value || "").trim().toLowerCase();
  if (!query) return state.dailyDetailDefinitions;
  return state.dailyDetailDefinitions.filter((item) => [item.key, item.label, item.scope, item.field_type].join(" ").toLowerCase().includes(query));
}

function renderDailyDetailDefinitionList() {
  if (!els.dailyDetailDefinitionList) return;
  const items = filteredDailyDetailDefinitions();
  if (items.length === 0) {
    els.dailyDetailDefinitionList.innerHTML = `<div class="log-item">Nu exista definitii pentru detalii zilnice.</div>`;
    return;
  }
  els.dailyDetailDefinitionList.innerHTML = `
    <div class="analyte-table-wrap">
      <table class="analyte-table">
        <thead>
          <tr>
            <th>Cheie</th>
            <th>Eticheta</th>
            <th>Scope</th>
            <th>Tip</th>
            <th>Sursa</th>
          </tr>
        </thead>
        <tbody>
          ${items.map((item) => `<tr class="${item.id === state.selectedDailyDetailDefinitionID ? "active" : ""}" data-daily-detail-definition-id="${item.id || 0}" data-daily-detail-definition-key="${escapeHtml(item.key || "")}">
            <td>${escapeHtml(item.key || "-")}</td>
            <td>${escapeHtml(item.label || "-")}</td>
            <td>${escapeHtml(item.scope || "-")}</td>
            <td>${escapeHtml(item.field_type || "-")}</td>
            <td>${escapeHtml(item.source || "static")}</td>
          </tr>`).join("")}
        </tbody>
      </table>
    </div>`;
  [...els.dailyDetailDefinitionList.querySelectorAll("[data-daily-detail-definition-key]")].forEach((row) => {
    row.addEventListener("click", () => {
      const key = String(row.dataset.dailyDetailDefinitionKey || "");
      const item = state.dailyDetailDefinitions.find((entry) => entry.key === key);
      if (!item) return;
      state.selectedDailyDetailDefinitionID = Number(item.id || 0) || null;
      state.selectedDailyDetailDefinitionKey = item.key || null;
      renderDailyDetailDefinitionList();
      openDailyDetailDefinitionModal(item);
    });
  });
}

function openDailyDetailDefinitionModal(item = null) {
  if (!els.dailyDetailDefinitionForm || !els.dailyDetailDefinitionModal) return;
  state.selectedDailyDetailDefinitionID = item?.id || null;
  state.selectedDailyDetailDefinitionKey = item?.key || null;
  const form = els.dailyDetailDefinitionForm;
  form.key.value = item?.key || "";
  form.label.value = item?.label || "";
  form.scope.value = item?.scope || "day";
  form.field_type.value = item?.field_type || "text";
  form.placeholder.value = item?.placeholder || "";
  form.default_value.value = item?.default_value || "";
  form.options_csv.value = Array.isArray(item?.options) ? item.options.join(", ") : "";
  form.sort_order.value = item?.sort_order ?? "";
  form.required.checked = !!item?.required;
  form.active.checked = item ? !!item.active : true;
  form.key.disabled = item?.source === "static";
  const saveBtn = document.getElementById("save-daily-detail-definition");
  if (saveBtn) saveBtn.disabled = item?.source === "static";
  els.deleteDailyDetailDefinitionBtn.hidden = !item || item?.source === "static";
  els.dailyDetailDefinitionMessage.textContent = item?.source === "static" ? "Definitiile statice pot fi doar consultate." : (item ? `Editing ${item.key}` : "Ready");
  els.dailyDetailDefinitionModal.hidden = false;
}

function closeDailyDetailDefinitionModal() {
  if (!els.dailyDetailDefinitionModal) return;
  els.dailyDetailDefinitionModal.hidden = true;
  state.selectedDailyDetailDefinitionID = null;
  state.selectedDailyDetailDefinitionKey = null;
  const saveBtn = document.getElementById("save-daily-detail-definition");
  if (saveBtn) saveBtn.disabled = false;
  if (els.dailyDetailDefinitionMessage) els.dailyDetailDefinitionMessage.textContent = "";
}

async function onRefreshDailyDetailDefinitionsClick() {
  try {
    await loadDailyDetailDefinitions();
  } catch (error) {
    showToast(error?.message || "Cannot load daily detail definitions", "error");
  }
}

async function onSaveDailyDetailDefinition(event) {
  event.preventDefault();
  const form = new FormData(els.dailyDetailDefinitionForm);
  const payload = {
    id: state.selectedDailyDetailDefinitionID || 0,
    key: String(form.get("key") || "").trim(),
    label: String(form.get("label") || "").trim(),
    scope: String(form.get("scope") || "day").trim(),
    field_type: String(form.get("field_type") || "text").trim(),
    placeholder: String(form.get("placeholder") || "").trim(),
    default_value: String(form.get("default_value") || "").trim(),
    options: String(form.get("options_csv") || "").split(",").map((item) => item.trim()).filter(Boolean),
    sort_order: Number(form.get("sort_order") || 0),
    required: !!form.get("required"),
    active: !!form.get("active"),
  };
  const url = payload.id ? `/api/daily-details/definitions/${payload.id}` : "/api/daily-details/definitions";
  const method = payload.id ? "PUT" : "POST";
  try {
    await api(url, { method, body: JSON.stringify(payload) });
    await loadDailyDetailDefinitions();
    closeDailyDetailDefinitionModal();
    showToast("Definitia a fost salvata.", "success");
  } catch (error) {
    els.dailyDetailDefinitionMessage.textContent = error?.message || "Cannot save definition";
    showToast(error?.message || "Cannot save definition", "error");
  }
}

async function onDeleteDailyDetailDefinition() {
  if (!state.selectedDailyDetailDefinitionID) return;
  if (!window.confirm("Stergi definitia selectata?")) return;
  try {
    await api(`/api/daily-details/definitions/${state.selectedDailyDetailDefinitionID}`, { method: "DELETE" });
    await loadDailyDetailDefinitions();
    closeDailyDetailDefinitionModal();
    showToast("Definitia a fost stearsa.", "success");
  } catch (error) {
    showToast(error?.message || "Cannot delete definition", "error");
  }
}

function filteredDailyDetailValueDefinitions() {
  const scope = state.dailyDetailsScopeTab || "day";
  const query = String(els.dailyDetailsValueSearch?.value || "").trim().toLowerCase();
  return (state.dailyDetailDefinitions || [])
    .filter((item) => item.active !== false && item.scope === scope)
    .filter((item) => !query || [item.key, item.label, item.placeholder].join(" ").toLowerCase().includes(query));
}

function renderDailyDetailValuesWorkspace() {
  const host = document.getElementById("daily-detail-values-list");
  if (!host) return;
  const scope = state.dailyDetailsScopeTab || "day";
  const usesRound = ["day_round", "day_round_analyte"].includes(scope);
  const usesAnalyte = ["day_analyte", "day_round_analyte"].includes(scope);
  if (usesAnalyte) {
    syncDailyDetailAnalyteOptions();
  }
  if (els.dailyDetailsRoundBox) els.dailyDetailsRoundBox.hidden = !usesRound;
  if (els.dailyDetailsAnalyteBox) els.dailyDetailsAnalyteBox.hidden = !usesAnalyte;
  els.dailyDetailsTabs.forEach((tab) => tab.classList.toggle("active", (tab.dataset.dailyScope || "day") === scope));
  const definitions = filteredDailyDetailValueDefinitions();
  if (definitions.length === 0) {
    host.innerHTML = `<div class="log-item">${escapeHtml(t("noDailyDetails"))}</div>`;
    return;
  }
  const roundNo = usesRound ? Number(state.dailyDetailsRoundNo || 0) : 0;
  const analyteTag = usesAnalyte ? String(state.dailyDetailsAnalyteTag || "").trim() : "";
  const valuesIndex = new Map((state.dailyDetailValues || []).map((item) => [`${item.definition_key}|${item.round_no || 0}|${item.analyte_tag || ""}`, item]));
  host.innerHTML = `
    <div class="analyte-table-wrap">
      <table class="analyte-table">
        <thead>
          <tr>
            <th>${escapeHtml(t("variableName"))}</th>
            <th>${escapeHtml(t("scope"))}</th>
            ${usesRound ? `<th>${escapeHtml(t("round"))}</th>` : ""}
            ${usesAnalyte ? `<th>${escapeHtml(t("navAnalytes"))}</th>` : ""}
            <th>${escapeHtml(t("value"))}</th>
            <th>${escapeHtml(t("source"))}</th>
          </tr>
        </thead>
        <tbody>
          ${definitions.map((item) => {
            const key = `${item.key}|${roundNo}|${analyteTag}`;
            const current = valuesIndex.get(key);
            const value = current?.value_text ?? item.default_value ?? "";
            const field = item.field_type === "select"
              ? `<select data-daily-detail-key="${escapeHtml(item.key)}" data-round-no="${roundNo}" data-analyte-tag="${escapeHtml(analyteTag)}">
                  <option value=""></option>
                  ${(item.options || []).map((option) => `<option value="${escapeHtml(option)}" ${option === value ? "selected" : ""}>${escapeHtml(option)}</option>`).join("")}
                </select>`
              : `<input type="${item.field_type === "number" ? "number" : "text"}" step="any" value="${escapeHtml(value)}" placeholder="${escapeHtml(item.placeholder || "")}" data-daily-detail-key="${escapeHtml(item.key)}" data-round-no="${roundNo}" data-analyte-tag="${escapeHtml(analyteTag)}">`;
            return `<tr>
              <td><strong>${escapeHtml(item.label || item.key)}</strong><div class="small muted">${escapeHtml(item.key || "-")}</div></td>
              <td>${escapeHtml(item.scope || "-")}</td>
              ${usesRound ? `<td>${escapeHtml(String(roundNo || "-"))}</td>` : ""}
              ${usesAnalyte ? `<td>${escapeHtml(analyteTag || "-")}</td>` : ""}
              <td>${field}</td>
              <td>${escapeHtml(item.source || "shared")}</td>
            </tr>`;
          }).join("")}
        </tbody>
      </table>
    </div>`;
  [...host.querySelectorAll("[data-daily-detail-key]")].forEach((input) => {
    const save = async () => {
      const payload = {
        definition_key: input.dataset.dailyDetailKey || "",
        scope_date: state.dailyDetailsDate || localISODate(),
        round_no: Number(input.dataset.roundNo || 0),
        analyte_tag: String(input.dataset.analyteTag || "").trim(),
        value_text: String(input.value || "").trim(),
      };
      try {
        await api("/api/daily-details", { method: "PUT", body: JSON.stringify(payload) });
        await loadDailyDetailValues(state.dailyDetailsDate, usesRound ? roundNo : 0);
        showToast(t("saved"), "success");
      } catch (error) {
        showToast(error?.message || "Cannot save daily detail", "error");
      }
    };
    input.addEventListener("change", save);
    input.addEventListener("blur", save);
  });
}

function fillAnalyteForm(item = null) {
  state.selectedAnalyteId = item?.id || null;
  state.selectedTag = item?.tag || null;
  const form = els.analyteForm;
  form.tag.value = item?.tag || "";
  form.code.value = item?.code || "";
  form.name.value = item?.name || "";
  form.description.value = item?.description || "";
  form.result_type.value = item?.result_type || "text";
  form.result_formatting.value = item?.result_formatting || "raw";
  form.result_weighting.value = item?.result_weighting ?? 1;
  form.result_measure_unit.value = item?.result_measure_unit || "";
  form.result_reagents_set.value = item?.result_reagents_set || "";
  form.worklist_label.value = item?.protocol_options?.worklist_label || "";
  form.active.checked = item ? !!item.active : true;
  els.deleteAnalyteBtn.hidden = !item;
  els.analyteMessage.textContent = item ? `${t("editing")} ${item.tag}` : t("readyNew");
  renderAnalyteList();
}

function openAnalyteModal(item = null) {
  fillAnalyteForm(item);
  els.analyteModal.hidden = false;
}

function closeAnalyteModal() {
  els.analyteModal.hidden = true;
  els.analyteMessage.textContent = "";
}

function openQCTargetModal(target = null) {
  syncQCConfiguredLevels();
  syncQCTargetFormAnalyteOptions();
  if (target) {
    state.editingQCTargetID = target.id || null;
    state.selectedQCTargetID = target.id || null;
    els.qcTargetAnalyte.value = target.analyte_tag || currentQCTargetAnalyteTag();
    els.qcTargetLevel.value = target.control_level || configuredQCLevels([target.control_level])[0] || "QC";
    els.qcTargetLot.value = target.lot_no || "-";
    els.qcTargetMean.value = target.target_mean ?? "";
    els.qcTargetSD.value = target.target_sd ?? "";
    updateQCTargetDerivedFields();
    els.qcTargetMessage.textContent = `${t("editing")} ${target.analyte_tag} / ${target.control_level} / ${target.lot_no || "-"}`;
  } else {
    state.editingQCTargetID = null;
    state.selectedQCTargetID = null;
    els.qcTargetAnalyte.value = currentQCTargetAnalyteTag();
    els.qcTargetLevel.value = configuredQCLevels()[0] || "QC";
    els.qcTargetLot.value = "-";
    els.qcTargetMean.value = "";
    els.qcTargetSD.value = "";
    updateQCTargetDerivedFields();
    els.qcTargetMessage.textContent = t("readyNew");
  }
  els.qcTargetModal.hidden = false;
}

function closeQCTargetModal() {
  els.qcTargetModal.hidden = true;
  state.editingQCTargetID = null;
  els.qcTargetMessage.textContent = "";
}

async function onRefreshQCTargetsClick() {
  try {
    await Promise.all([loadQCMeta(), loadAnalytes(), loadQCTargets()]);
  } catch (error) {
    showToast(error?.message || "Cannot load QC settings", "error");
  }
}

async function onSaveAnalyte(event) {
  event.preventDefault();
  const form = new FormData(els.analyteForm);
  const payload = {
    id: state.selectedAnalyteId || 0,
    tag: String(form.get("tag") || "").trim(),
    code: String(form.get("code") || "").trim(),
    name: String(form.get("name") || "").trim(),
    description: String(form.get("description") || "").trim(),
    result_type: String(form.get("result_type") || "text").trim(),
    result_formatting: String(form.get("result_formatting") || "raw").trim(),
    result_weighting: Number(form.get("result_weighting") || 1),
    result_measure_unit: String(form.get("result_measure_unit") || "").trim(),
    result_reagents_set: String(form.get("result_reagents_set") || "").trim(),
    protocol_options: {
      worklist_label: String(form.get("worklist_label") || "").trim(),
    },
    active: !!form.get("active"),
  };
  const method = state.selectedAnalyteId ? "PUT" : "POST";
  const url = state.selectedAnalyteId ? `/api/analytes/${state.selectedAnalyteId}` : "/api/analytes";
  setAnalyteSaveState(true);
  try {
    const resp = await api(url, { method, body: JSON.stringify(payload) });
    await loadAnalytes();
    const savedID = Number(resp.id || payload.id || 0);
    const saved = state.analytes.find((item) => item.id === savedID) || state.analytes.find((item) => item.tag === payload.tag);
    fillAnalyteForm(saved || null);
    els.analyteMessage.textContent = t("saved");
    showToast(`${t("saved")} ${payload.tag}`, "success");
  } catch (error) {
    els.analyteMessage.textContent = error?.message || "Cannot save analyte";
    showToast(error?.message || "Cannot save analyte", "error");
  } finally {
    setAnalyteSaveState(false);
  }
}

async function onDeleteAnalyte() {
  if (!state.selectedAnalyteId) return;
  if (!window.confirm(`${t("deleteConfirm")} ${state.selectedTag}?`)) return;
  try {
    const deletedTag = state.selectedTag;
    await api(`/api/analytes/${state.selectedAnalyteId}`, { method: "DELETE" });
    state.selectedAnalyteId = null;
    state.selectedTag = null;
    fillAnalyteForm();
    await loadAnalytes();
    closeAnalyteModal();
    showToast(`${t("delete")} ${deletedTag}`, "success");
  } catch (error) {
    els.analyteMessage.textContent = error?.message || "Cannot delete analyte";
    showToast(error?.message || "Cannot delete analyte", "error");
  }
}

async function onSaveQCTarget(event) {
  event.preventDefault();
  const analyteTag = String(els.qcTargetAnalyte.value || "").trim();
  const analyte = state.analytes.find((item) => item.tag === analyteTag) || null;
  if (!analyte) {
    showToast("Selecteaza o analiza", "error");
    return;
  }
  const payload = {
    id: state.editingQCTargetID || state.selectedQCTargetID || 0,
    active: true,
    analyte_tag: analyte.tag,
    analyte_name: analyte.name || analyte.tag,
    control_level: String(els.qcTargetLevel.value || "QC").trim(),
    lot_no: String(els.qcTargetLot.value || "-").trim() || "-",
    unit: analyte.result_measure_unit || "",
    target_mean: Number(els.qcTargetMean.value || 0),
    target_sd: Number(els.qcTargetSD.value || 0),
  };
  const method = payload.id ? "PUT" : "POST";
  const url = payload.id ? `/api/qc-targets/${payload.id}` : "/api/qc-targets";
  try {
    await api(url, { method, body: JSON.stringify(payload) });
    state.selectedQCTargetAnalyteTag = payload.analyte_tag;
    state.selectedQCTargetID = Number(payload.id || 0) || state.selectedQCTargetID;
    await loadQCTargets();
    closeQCTargetModal();
    showToast(`${t("saveQcTarget")} ${payload.analyte_tag}`, "success");
  } catch (error) {
    els.qcTargetMessage.textContent = error.message;
    showToast(error.message, "error");
  }
}

async function onDeleteQCTarget() {
  const targetID = state.editingQCTargetID || state.selectedQCTargetID;
  if (!targetID) return;
  const target = state.qcTargets.find((item) => item.id === targetID);
  if (!window.confirm(`${t("delete")} ${target?.analyte_tag || ""} / ${target?.lot_no || "-" }?`)) return;
  try {
    await api(`/api/qc-targets/${targetID}`, { method: "DELETE" });
    state.selectedQCTargetID = null;
    state.editingQCTargetID = null;
    await loadQCTargets();
    closeQCTargetModal();
    showToast(t("delete"), "success");
  } catch (error) {
    showToast(error.message, "error");
  }
}

function renderWestgardSummaryInto(metrics, summaryEl, rulesEl) {
  if (!metrics || !Number(metrics.count || 0)) {
    summaryEl.innerHTML = `<div class="log-item">${escapeHtml(t("noWestgard"))}</div>`;
    rulesEl.innerHTML = "";
    els.qcWestgardValidateBtn.hidden = true;
    return;
  }
  const cards = [
    { label: "N", value: metrics.count ?? 0 },
    { label: t("qcTargetMean"), value: formatMetricNumber(metrics.target_mean) },
    { label: t("qcTargetSD"), value: formatMetricNumber(metrics.target_sd) },
    { label: t("qcTargetCV"), value: formatMetricNumber(metrics.target_cv) },
    { label: "Mean", value: formatMetricNumber(metrics.mean) },
    { label: "SD", value: formatMetricNumber(metrics.sd) },
    { label: "CV", value: formatMetricNumber(metrics.cv) },
    { label: t("westgardMedian"), value: formatMetricNumber(metrics.median) },
    { label: t("westgardOutliers"), value: metrics.outliers ?? 0 },
    { label: t("westgardRepeatability"), value: formatMetricNumber(metrics.repeatability) },
    ...(metrics.use_own_mean ? [{ label: t("westgardStatsOwnMean"), value: formatMetricNumber(metrics.robust_mean) }] : []),
    { label: "1_2s", value: metrics.westgard?.["1_2s"] ?? 0 },
    { label: "1_3s", value: metrics.westgard?.["1_3s"] ?? 0 },
    { label: "2_2s", value: metrics.westgard?.["2_2s"] ?? 0 },
    { label: "R_4s", value: metrics.westgard?.["r_4s"] ?? 0 },
    { label: "4_1s", value: metrics.westgard?.["4_1s"] ?? 0 },
    { label: "10x", value: metrics.westgard?.["10x"] ?? 0 },
    { label: "7T", value: metrics.westgard?.["7t"] ?? 0 },
  ];
  summaryEl.innerHTML = cards.map((item) => `<div class="metric"><span class="muted small">${escapeHtml(String(item.label))}</span><strong>${escapeHtml(String(item.value))}</strong></div>`).join("");
  const rules = Array.isArray(metrics.westgard?.rules) ? metrics.westgard.rules : [];
  rulesEl.innerHTML = rules.map((item) => `<div class="legend-item"><span class="legend-swatch" style="background:#d63a48"></span><span>${escapeHtml(`${item.run_date} · ${item.control_label || "-"} · ${item.rules.join(", ")}`)}</span></div>`).join("");
  const issues = Array.isArray(metrics.validation_issues) ? metrics.validation_issues : [];
  els.qcWestgardValidateBtn.hidden = false;
  els.qcWestgardValidateBtn.textContent = issues.length ? t("westgardInvalid") : t("westgardValid");
  els.qcWestgardValidateBtn.className = issues.length ? "danger" : "ghost";
}

function renderWestgardChartInto(metrics, chartEl) {
  const points = Array.isArray(metrics?.points) ? metrics.points : [];
  if (points.length === 0) {
    chartEl.innerHTML = "";
    return;
  }
  const width = 960;
  const height = 320;
  const padL = 60;
  const padR = 24;
  const padT = 20;
  const padB = 42;
  const targetMean = Number(metrics.target_mean || 0);
  const targetSD = Number(metrics.target_sd || 0);
  const values = points.map((item) => Number(item.value || 0));
  const bounds = [targetMean - 3 * targetSD, targetMean + 3 * targetSD, ...values];
  const minVal = Math.min(...bounds);
  const maxVal = Math.max(...bounds);
  const span = maxVal - minVal || 1;
  const x = (index) => padL + index * ((width - padL - padR) / Math.max(points.length - 1, 1));
  const y = (value) => height - padB - (((value - minVal) / span) * (height - padT - padB));
  const guide = (value, label, color, dash = "4 4") => `<line x1="${padL}" y1="${y(value)}" x2="${width - padR}" y2="${y(value)}" stroke="${color}" stroke-width="1.2" stroke-dasharray="${dash}"/><text x="${padL}" y="${y(value) - 6}" fill="${color}" font-size="11">${escapeHtml(label)}</text>`;
  const path = points.map((item, index) => `${index === 0 ? "M" : "L"} ${x(index)} ${y(Number(item.value || 0))}`).join(" ");
  chartEl.innerHTML = `
    <rect x="0" y="0" width="${width}" height="${height}" fill="#ffffff"></rect>
    ${guide(targetMean, "Mean", "#14786b", "0")}
    ${targetSD > 0 ? guide(targetMean + targetSD, "+1SD", "#1e9a8a") : ""}
    ${targetSD > 0 ? guide(targetMean - targetSD, "-1SD", "#1e9a8a") : ""}
    ${targetSD > 0 ? guide(targetMean + 2 * targetSD, "+2SD", "#d98a1f") : ""}
    ${targetSD > 0 ? guide(targetMean - 2 * targetSD, "-2SD", "#d98a1f") : ""}
    ${targetSD > 0 ? guide(targetMean + 3 * targetSD, "+3SD", "#d63a48") : ""}
    ${targetSD > 0 ? guide(targetMean - 3 * targetSD, "-3SD", "#d63a48") : ""}
    <path d="${path}" fill="none" stroke="#229148" stroke-width="2.4"></path>
    ${points.map((item, index) => {
      const rules = Array.isArray(item.westgard_rules) ? item.westgard_rules : [];
      const color = rules.length ? "#d63a48" : "#229148";
      return `<circle cx="${x(index)}" cy="${y(Number(item.value || 0))}" r="4.5" fill="${color}">
        <title>${escapeHtml(`${item.run_date} · ${item.control_label || "-"} · ${item.value}`)}${rules.length ? escapeHtml(` · ${rules.join(", ")}`) : ""}</title>
      </circle><text x="${x(index)}" y="${height - 14}" text-anchor="middle" fill="#5f6e67" font-size="10">${index + 1}</text>`;
    }).join("")}
  `;
}

async function onGenerateQCWestgard() {
  const bundle = state.qcRecords.find((item) => item.record.id === state.selectedQCRecordId);
  const selectedAnalysis = bundle?.analyses?.find((item) => item.id === state.selectedQCAnalysisId) || bundle?.analyses?.find((item) => item.analyte_tag === state.selectedQCAnalysisTag);
  if (!bundle || !selectedAnalysis) {
    return;
  }
  syncQCWestgardPeriodControls();
  const { from, to } = qcWestgardRange();
  const params = new URLSearchParams({
    analyte_tag: selectedAnalysis.analyte_tag || "",
    control_level: selectedAnalysis.control_level || bundle.record.control_level || "",
    lot_no: (selectedAnalysis.lot_no || bundle.record.lot_no || "-").trim() || "-",
    date_from: from,
    date_to: to,
  });
  const resp = await api(`/api/qc/metrics?${params.toString()}`);
  state.qcWestgardMetrics = resp.metrics || null;
  els.qcWestgardModal.hidden = false;
  renderWestgardSummaryInto(state.qcWestgardMetrics, els.qcWestgardSummary, els.qcWestgardRules);
  renderWestgardChartInto(state.qcWestgardMetrics, els.qcWestgardChart);
}

function closeQCWestgardModal() {
  els.qcWestgardModal.hidden = true;
}

function syncQCWestgardPeriodControls() {
  if (!els.qcWestgardPeriod) return;
  if (!els.qcWestgardPeriod.options.length) {
    els.qcWestgardPeriod.innerHTML = [
      ["current_week", t("westgardCurrentWeek")],
      ["previous_week", t("westgardPreviousWeek")],
      ["current_month", t("westgardCurrentMonth")],
      ["previous_month", t("westgardPreviousMonth")],
      ["current_year", t("westgardCurrentYear")],
      ["custom", t("westgardCustom")],
    ].map(([value, label]) => `<option value="${escapeHtml(value)}">${escapeHtml(label)}</option>`).join("");
  }
  els.qcWestgardPeriod.value = state.qcWestgardPeriod || "current_month";
  const isCustom = els.qcWestgardPeriod.value === "custom";
  const bounds = currentPeriodBounds(els.qcWestgardPeriod.value || "current_month");
  if (!isCustom) {
    els.qcWestgardDateFrom.value = bounds.from;
    els.qcWestgardDateTo.value = bounds.to;
  }
  els.qcWestgardDateFrom.disabled = !isCustom;
  els.qcWestgardDateTo.disabled = !isCustom;
}

function qcWestgardRange() {
  if ((state.qcWestgardPeriod || "current_month") === "custom") {
    return {
      from: els.qcWestgardDateFrom.value || localISODate(),
      to: els.qcWestgardDateTo.value || localISODate(),
    };
  }
  return currentPeriodBounds(state.qcWestgardPeriod || "current_month");
}

function formatMetricNumber(value) {
  const num = Number(value);
  if (!Number.isFinite(num)) return "-";
  return num.toFixed(4).replace(/\.?0+$/, "");
}

function renderOrdersLayout() {
  if (state.orders.length === 0) {
    els.ordersSelectAll.checked = false;
    els.ordersLayout.innerHTML = `<div class="log-item">${escapeHtml(t("noOrders"))}</div>`;
    return;
  }
  const layoutKind = inferLayoutKind();
  if (layoutKind === "rack_positions") {
    const racks = new Map();
    state.orders.forEach((bundle) => {
      const rackNo = bundle.order.rack_no || 1;
      if (!racks.has(rackNo)) racks.set(rackNo, []);
      racks.get(rackNo).push(bundle);
    });
    els.ordersLayout.innerHTML = `<div class="rack-grid">${[...racks.entries()].sort((a, b) => a[0] - b[0]).map(([rackNo, items]) => `
      <div class="rack-card">
        <div class="rack-title">Rack ${rackNo}</div>
        <div class="slot-grid">${items.sort((a, b) => (a.order.rack_position || 0) - (b.order.rack_position || 0)).map((bundle) => renderSlot(bundle)).join("")}</div>
      </div>`).join("")}</div>`;
  } else {
    els.ordersLayout.innerHTML = `
      <div class="orders-table-wrap">
        <table class="orders-table">
          <thead>
            <tr>
              <th class="col-check"></th>
              <th class="col-slot">${escapeHtml(t("slot"))}</th>
              <th class="col-sample">${escapeHtml(t("sample"))}</th>
              <th class="col-status">${escapeHtml(t("status"))}</th>
              <th class="col-analyses">${escapeHtml(t("analyses"))}</th>
            </tr>
          </thead>
          <tbody>${state.orders.sort((a, b) => {
            const aSampleNo = a.order.sample_no || 0;
            const bSampleNo = b.order.sample_no || 0;
            if (aSampleNo !== bSampleNo) return aSampleNo - bSampleNo;
            return (a.order.id || 0) - (b.order.id || 0);
          }).map((bundle) => renderSimpleRow(bundle)).join("")}</tbody>
        </table>
      </div>`;
  }
  [...els.ordersLayout.querySelectorAll("[data-order-id]")].forEach((button) => {
    button.addEventListener("click", () => {
      state.selectedOrderId = Number(button.dataset.orderId);
      const bundle = state.orders.find((item) => item.order.id === state.selectedOrderId);
      state.selectedOrderAnalysisID = bundle?.analyses?.[0]?.analysis?.id || null;
      renderOrdersLayout();
      renderOrderDetails();
    });
  });
  [...els.ordersLayout.querySelectorAll("[data-order-check]")].forEach((checkbox) => {
    checkbox.addEventListener("click", (event) => {
      event.stopPropagation();
    });
    checkbox.addEventListener("change", () => {
      const id = Number(checkbox.dataset.orderCheck || 0);
      if (!id) return;
      if (checkbox.checked) {
        if (!state.selectedOrderIDs.includes(id)) state.selectedOrderIDs.push(id);
      } else {
        state.selectedOrderIDs = state.selectedOrderIDs.filter((item) => item !== id);
      }
      els.ordersSelectAll.checked = state.orders.length > 0 && state.orders.every((item) => state.selectedOrderIDs.includes(item.order.id));
    });
  });
  els.ordersSelectAll.checked = state.orders.length > 0 && state.orders.every((item) => state.selectedOrderIDs.includes(item.order.id));
}

function renderRoundSelect() {
  if (!els.roundSelect) return;
  els.roundSelect.innerHTML = state.rounds.map((roundNo) => `<option value="${roundNo}">${roundNo}</option>`).join("");
  els.roundSelect.value = String(state.selectedRoundNo > 0 ? state.selectedRoundNo : (state.rounds[0] || 1));
}

function renderQCRoundSelect() {
  if (!els.qcRoundSelect) return;
  els.qcRoundSelect.innerHTML = state.qcRounds.map((roundNo) => `<option value="${roundNo}">${roundNo}</option>`).join("");
  els.qcRoundSelect.value = String(state.selectedQCRoundNo > 0 ? state.selectedQCRoundNo : (state.qcRounds[0] || 1));
}

function knownAnalyteOptions() {
  const seen = new Set();
  const options = [];
  const push = (tag, name = "") => {
    const normalizedTag = String(tag || "").trim();
    if (!normalizedTag || seen.has(normalizedTag)) return;
    seen.add(normalizedTag);
    options.push({ tag: normalizedTag, name: String(name || "").trim() });
  };
  state.analytes.filter((item) => item.active !== false).forEach((item) => push(item.tag, item.name));
  state.qcTargets.forEach((item) => push(item.analyte_tag, item.analyte_name));
  state.qcRecords.forEach((bundle) => (bundle.analyses || []).forEach((item) => push(item.analyte_tag, item.analyte_name)));
  return options.sort((a, b) => a.tag.localeCompare(b.tag));
}

function renderQCAnalyteFilter() {
  if (!els.qcAnalyteFilter) return;
  const analytes = knownAnalyteOptions();
  if (state.selectedQCAnalyteFilter && !analytes.some((item) => item.tag === state.selectedQCAnalyteFilter)) {
    state.selectedQCAnalyteFilter = "";
  }
  els.qcAnalyteFilter.innerHTML = [`<option value="">${escapeHtml(t("allAnalytes"))}</option>`, ...analytes.map((item) => `<option value="${escapeHtml(item.tag)}">${escapeHtml(item.tag)}${item.name ? ` · ${escapeHtml(item.name)}` : ""}</option>`)].join("");
  els.qcAnalyteFilter.value = state.selectedQCAnalyteFilter || "";
}

function renderQCLevelFilter() {
  if (!els.qcLevelFilter) return;
  const levels = configuredQCLevels([
    ...state.qcRecords.map((item) => String(item.record.control_level || "").trim()),
    ...state.qcRecords.flatMap((item) => (item.analyses || []).map((analysis) => String(analysis.control_level || "").trim())),
  ]).sort((a, b) => a.localeCompare(b));
  els.qcLevelFilter.innerHTML = [`<option value="">${escapeHtml(t("allControlTypes"))}</option>`, ...levels.map((level) => `<option value="${escapeHtml(level)}">${escapeHtml(level)}</option>`)].join("");
  els.qcLevelFilter.value = state.selectedQCLevel || "";
}

function filteredQCRecords() {
  return state.qcRecords.filter((item) => {
    const levelOk = !state.selectedQCLevel || String(item.record.control_level || "").trim() === state.selectedQCLevel;
    const analyteOk = !state.selectedQCAnalyteFilter || (item.analyses || []).some((entry) => String(entry.analyte_tag || "").trim() === state.selectedQCAnalyteFilter);
    return levelOk && analyteOk;
  });
}

function filteredQCRows() {
  return filteredQCRecords().flatMap((bundle) => {
    const analyses = (bundle.analyses || []).filter((analysis) => {
      const levelOk = !state.selectedQCLevel || String((analysis.control_level || bundle.record.control_level || "")).trim() === state.selectedQCLevel;
      const analyteOk = !state.selectedQCAnalyteFilter || String(analysis.analyte_tag || "").trim() === state.selectedQCAnalyteFilter;
      return levelOk && analyteOk;
    });
    return analyses.map((analysis) => ({ record: bundle.record, analysis }));
  });
}

function renderSlot(bundle) {
  const order = bundle.order;
  const analyses = bundle.analyses || [];
  const summary = analyses.map((item) => item.analysis.analyte_tag).slice(0, 2).join(", ");
  return `<button class="slot filled" data-order-id="${order.id}" type="button">
    <span class="slot-index">${escapeHtml(slotLabel(order))}</span>
    <span class="slot-value">${escapeHtml(order.sample_id || "-")}</span>
    <span class="small muted">${escapeHtml(summary || order.status || "")}</span>
  </button>`;
}

function renderSimpleRow(bundle) {
  const order = bundle.order;
  const analysesCount = (bundle.analyses || []).length;
  return `<tr class="${order.id === state.selectedOrderId ? "active" : ""}" data-order-id="${order.id}">
    <td class="col-check">
      ${state.commType === "file" ? `<span class="order-select"><input type="checkbox" data-order-check="${order.id}" ${state.selectedOrderIDs.includes(order.id) ? "checked" : ""}></span>` : ""}
    </td>
    <td class="col-slot"><span class="slot-pill">${escapeHtml(slotLabel(order))}</span></td>
    <td class="col-sample">
        <div class="sample-main">
          <div class="sample-id">${escapeHtml(order.sample_id || "-")}</div>
        <div class="sample-sub">
          <span>${escapeHtml(`${t("sampleNo")}: ${String(order.sample_no || 0)}`)}</span>
          <span>${escapeHtml(order.patient_name || "")}</span>
        </div>
      </div>
    </td>
    <td class="col-status"><span class="status-pill-soft">${escapeHtml(localizeOrderStatus(order.status || ""))}</span></td>
    <td class="col-analyses"><span class="count-pill">${escapeHtml(String(analysesCount))}</span></td>
  </tr>`;
}

function slotLabel(order) {
  if (inferLayoutKind() === "rack_positions") {
    return `R${order.rack_no || 1} · P${order.rack_position || 0}`;
  }
  return `#${order.sample_no || 0}`;
}

function renderOrderDetails() {
  const bundle = state.orders.find((item) => item.order.id === state.selectedOrderId);
  if (!bundle) {
    els.orderDetails.innerHTML = `<div class="log-item">${escapeHtml(t("orderDetails"))}</div>`;
    return;
  }
  const analyses = bundle.analyses || [];
  if (!state.selectedOrderAnalysisID && analyses.length > 0) {
    state.selectedOrderAnalysisID = analyses[0].analysis.id;
  }
  const selectedAnalysisBundle = analyses.find((item) => item.analysis.id === state.selectedOrderAnalysisID) || analyses[0] || null;
  els.orderDetails.innerHTML = `
    <div class="order-card">
      <div class="order-headline">
        <div class="order-title">
          <strong>${escapeHtml(bundle.order.sample_id)}</strong>
          <div class="small muted">${escapeHtml(bundle.order.patient_name || "")}</div>
        </div>
        <span class="slot-pill">${escapeHtml(`${t("round")} ${bundle.order.round_no || 1}`)}</span>
      </div>
      <div class="order-meta-grid">
        <div class="meta-kpi">
          <span class="label">${escapeHtml(t("sampleNo"))}</span>
          <span class="value">${escapeHtml(String(bundle.order.sample_no || 0))}</span>
        </div>
        <div class="meta-kpi">
          <span class="label">${escapeHtml(t("orderDate"))}</span>
          <span class="value">${escapeHtml(bundle.order.order_date || "-")}</span>
        </div>
        <div class="meta-kpi">
          <span class="label">${escapeHtml(t("status"))}</span>
          <span class="value">${escapeHtml(localizeOrderStatus(bundle.order.status || ""))}</span>
        </div>
      </div>
    </div>
    <div class="analysis-list">
      <div class="analysis-card">
        <strong>${escapeHtml(t("analysesForSample"))}</strong>
        ${analyses.length > 0 ? `
          <div class="analysis-table-wrap">
            <table class="analysis-table">
              <thead>
                <tr>
                  <th>${escapeHtml(t("analysisName"))}</th>
                  <th>${escapeHtml(t("analysisTag"))}</th>
                  <th>${escapeHtml(t("analysisQualitative"))}</th>
                  <th>${escapeHtml(t("analysisQuantitative"))}</th>
                </tr>
              </thead>
              <tbody>
                ${analyses.map((item) => `
                  <tr class="${item.analysis.id === state.selectedOrderAnalysisID ? "active" : ""}" data-analysis-id="${item.analysis.id}">
                    <td>
                      <strong>${escapeHtml(item.analysis.analyte_name || item.analysis.analyte_tag || "-")}</strong>
                      <div class="small muted">${escapeHtml(item.analysis.analyte_description || "")}</div>
                    </td>
                    <td>${escapeHtml(item.analysis.analyte_tag || "-")}</td>
                    <td>${escapeHtml(item.analysis.result_value || t("noResult"))}</td>
                    <td>${escapeHtml(item.analysis.raw_value || "-")}</td>
                  </tr>`).join("")}
              </tbody>
            </table>
          </div>` : `<div class="log-item">${escapeHtml(t("noAnalytes"))}</div>`}
      </div>
      ${selectedAnalysisBundle ? `
        <div class="analysis-card">
          <div class="analysis-header">
            <div class="analysis-title">
              <strong>${escapeHtml(selectedAnalysisBundle.analysis.analyte_tag)}</strong>
              <div class="small muted">${escapeHtml(selectedAnalysisBundle.analysis.analyte_name || "")}</div>
              <div class="small muted">${escapeHtml(selectedAnalysisBundle.analysis.analyte_description || "")}</div>
            </div>
            <div class="analysis-value-box">
              <div class="small muted">${escapeHtml(t("currentResult"))}</div>
              <strong>${escapeHtml(selectedAnalysisBundle.analysis.result_value || t("noResult"))}</strong>
              <div class="small muted">${escapeHtml(`${t("analysisQuantitative")}: ${selectedAnalysisBundle.analysis.raw_value || "-"}`)}</div>
              <div class="small muted">${escapeHtml(selectedAnalysisBundle.analysis.interpreted_value || "")}</div>
            </div>
          </div>
          <label class="stack">
            <span>${escapeHtml(t("otherResults"))}</span>
            <select data-default-analysis-id="${selectedAnalysisBundle.analysis.id}">
              <option value="">${escapeHtml(t("selectResult"))}</option>
              ${(selectedAnalysisBundle.results || []).map((result) => `<option value="${result.id}" ${result.id === selectedAnalysisBundle.analysis.default_result_id ? "selected" : ""}>${escapeHtml(result.result_value || t("noResult"))} · ${escapeHtml(result.raw_value || "-")} · ${escapeHtml(formatDate(result.created_at))}</option>`).join("")}
            </select>
          </label>
        </div>` : ""}
    </div>`;
  [...els.orderDetails.querySelectorAll("[data-analysis-id]")].forEach((row) => {
    row.addEventListener("click", () => {
      state.selectedOrderAnalysisID = Number(row.dataset.analysisId || 0) || null;
      renderOrderDetails();
    });
  });
  [...els.orderDetails.querySelectorAll("[data-default-analysis-id]")].forEach((select) => {
    select.addEventListener("change", async () => {
      const resultID = Number(select.value || 0);
      const orderAnalysisID = Number(select.dataset.defaultAnalysisId || 0);
      if (!resultID || !orderAnalysisID) return;
      await api("/api/results/default", {
        method: "PUT",
        body: JSON.stringify({ order_analysis_id: orderAnalysisID, result_id: resultID }),
      });
      await loadOrders();
      });
  });
}

function renderQCLayout() {
  renderQCLevelFilter();
  renderQCAnalyteFilter();
  const items = filteredQCRows();
  if (items.length === 0) {
    els.qcLayout.innerHTML = `<div class="log-item">${escapeHtml(t("noQc"))}</div>`;
    return;
  }
  els.qcLayout.innerHTML = `
    <div class="orders-table-wrap">
      <table class="orders-table">
        <thead>
          <tr>
            <th class="col-slot">${escapeHtml(t("qcDate"))}</th>
            <th>${escapeHtml(t("analysisName"))}</th>
            <th>${escapeHtml(t("lotNo"))}</th>
            <th>${escapeHtml(t("analysisQuantitative"))}</th>
            <th class="col-status">${escapeHtml(t("qcLevel"))}</th>
          </tr>
        </thead>
        <tbody>${items.map((row) => renderQCRow(row)).join("")}</tbody>
      </table>
    </div>`;
  [...els.qcLayout.querySelectorAll("[data-qc-record-id][data-qc-analysis-id]")].forEach((row) => {
    row.addEventListener("click", () => {
      state.selectedQCRecordId = Number(row.dataset.qcRecordId || 0);
      state.selectedQCAnalysisId = Number(row.dataset.qcAnalysisId || 0) || null;
      state.selectedQCAnalysisTag = row.dataset.qcAnalysisTag || null;
      state.qcWestgardMetrics = null;
      renderQCLayout();
      renderQCDetails();
    });
  });
}

function renderQCRow(row) {
  const record = row.record;
  const analysis = row.analysis;
  const isActive = record.id === state.selectedQCRecordId && analysis?.id === state.selectedQCAnalysisId;
  return `<tr class="${isActive ? "active" : ""}" data-qc-record-id="${record.id}" data-qc-analysis-id="${analysis?.id || 0}" data-qc-analysis-tag="${escapeHtml(analysis?.analyte_tag || "")}">
    <td class="col-slot"><span class="slot-pill">${escapeHtml(formatDate(analysis?.created_at || record.created_at || record.run_date || "-"))}</span></td>
    <td><strong>${escapeHtml(analysis?.analyte_name || analysis?.analyte_tag || "-")}</strong><div class="small muted">${escapeHtml(analysis?.analyte_tag || "-")}</div></td>
    <td>${escapeHtml(analysis?.lot_no || record.lot_no || "-")}</td>
    <td>${escapeHtml(analysis?.raw_value || "-")}</td>
    <td class="col-status"><span class="status-pill-soft">${escapeHtml(analysis?.control_level || record.control_level || "-")}</span></td>
  </tr>`;
}

function renderQCDetails() {
  const bundle = state.qcRecords.find((item) => item.record.id === state.selectedQCRecordId);
  if (!bundle) {
    els.qcDetails.innerHTML = `<div class="log-item">${escapeHtml(t("qcDetails"))}</div>`;
    els.qcOpenWestgardBtn.disabled = true;
    return;
  }
  const analyses = (bundle.analyses || []).filter((item) => {
    const levelOk = !state.selectedQCLevel || String((item.control_level || bundle.record.control_level || "")).trim() === state.selectedQCLevel;
    const analyteOk = !state.selectedQCAnalyteFilter || String(item.analyte_tag || "").trim() === state.selectedQCAnalyteFilter;
    return levelOk && analyteOk;
  });
  const selectedAnalysis = analyses.find((item) => item.id === state.selectedQCAnalysisId) || analyses[0] || null;
  if (selectedAnalysis) {
    state.selectedQCAnalysisId = selectedAnalysis.id;
    state.selectedQCAnalysisTag = selectedAnalysis.analyte_tag || null;
  }
  els.qcOpenWestgardBtn.disabled = !selectedAnalysis;
  els.qcDetails.innerHTML = `
    <div class="order-card">
      <div class="order-headline">
        <div class="order-title">
          <strong>${escapeHtml(bundle.record.control_label || "-")}</strong>
          <div class="small muted">${escapeHtml(`${t("qcLevel")}: ${bundle.record.control_level || "-"}`)}</div>
        </div>
        <span class="slot-pill">${escapeHtml(bundle.record.run_date || "-")}</span>
      </div>
      <div class="order-meta-grid">
        <div class="meta-kpi"><span class="label">${escapeHtml(t("lotNo"))}</span><span class="value">${escapeHtml(selectedAnalysis?.lot_no || bundle.record.lot_no || "-")}</span></div>
        <div class="meta-kpi"><span class="label">${escapeHtml(t("qcLevel"))}</span><span class="value">${escapeHtml(selectedAnalysis?.control_level || bundle.record.control_level || "-")}</span></div>
        <div class="meta-kpi"><span class="label">${escapeHtml(t("qcReadAt"))}</span><span class="value">${escapeHtml(formatDate(selectedAnalysis?.created_at || bundle.record.created_at || bundle.record.run_date || "-"))}</span></div>
      </div>
      <div class="analysis-table-wrap">
        <table class="analysis-table">
          <thead>
            <tr>
              <th>${escapeHtml(t("qcReadAt"))}</th>
              <th>${escapeHtml(t("analysisName"))}</th>
              <th>${escapeHtml(t("lotNo"))}</th>
              <th>${escapeHtml(t("analysisQuantitative"))}</th>
              <th>${escapeHtml(t("qcLevel"))}</th>
            </tr>
          </thead>
          <tbody>
            ${analyses.map((item) => `
              <tr class="${item.id === state.selectedQCAnalysisId ? "active" : ""}" data-qc-analysis-id="${item.id}" data-qc-analysis-tag="${escapeHtml(item.analyte_tag || "")}">
                <td>${escapeHtml(formatDate(item.created_at || bundle.record.created_at || bundle.record.run_date || "-"))}</td>
                <td>
                  <strong>${escapeHtml(item.analyte_name || item.analyte_tag || "-")}</strong>
                </td>
                <td>${escapeHtml(item.lot_no || "-")}</td>
                <td>${escapeHtml(item.raw_value || "-")}</td>
                <td>${escapeHtml(item.control_level || "-")}</td>
              </tr>`).join("")}
          </tbody>
        </table>
      </div>
    </div>`;
  [...els.qcDetails.querySelectorAll("[data-qc-analysis-id]")].forEach((row) => {
    row.addEventListener("click", () => {
      state.selectedQCAnalysisId = Number(row.dataset.qcAnalysisId || 0) || null;
      state.selectedQCAnalysisTag = row.dataset.qcAnalysisTag || null;
      renderQCDetails();
      renderQCLayout();
    });
  });
}

function inferLayoutKind() {
  return state.layoutKind || "simple_list";
}

async function api(url, options = {}) {
  const response = await fetch(url, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok || payload.ok === false) {
    throw new Error(payload.error || `Request failed with ${response.status}`);
  }
  return payload;
}

function formatDate(value) {
  if (!value) return "-";
  const dt = new Date(value);
  if (Number.isNaN(dt.getTime())) return value;
  return dt.toLocaleString(state.language === "en" ? "en-GB" : "ro-RO");
}

function formatLogPayload(payload) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload) && payload.length === 0) {
    return "";
  }
  try {
    return JSON.stringify(payload, null, 2);
  } catch (_) {
    return "";
  }
}

function t(key) {
  return translations[state.language]?.[key] || translations.ro[key] || key;
}

function dataI18nKey(key) {
  const explicit = {
    nav_overview: "navOverview",
    nav_daily_details: "navDailyDetails",
    nav_analytes: "navSettings",
    nav_orders: "navOrders",
    nav_qc: "navQc",
    nav_help: "navHelp",
    settings_analytes: "settingsAnalytes",
    settings_daily_details: "settingsDailyDetails",
    settings_qc: "settingsQc",
    search: "search",
    daily_details_day: "dailyDetailsDayTab",
    daily_details_day_round: "dailyDetailsDayRoundTab",
    daily_details_day_analyte: "dailyDetailsDayAnalyteTab",
    daily_details_day_round_analyte: "dailyDetailsDayRoundAnalyteTab",
    field_tag: "fieldTag",
    field_code: "fieldCode",
    field_name: "fieldName",
    field_description: "fieldDescription",
    field_result_type: "fieldResultType",
    field_formatting: "fieldFormatting",
    field_weighting: "fieldWeighting",
    field_measure_unit: "fieldMeasureUnit",
    field_reagents_set: "fieldReagentsSet",
    field_active: "fieldActive",
  };
  return explicit[key] || key;
}

function syncLanguageControls() {
  els.languageSelectLogin.value = state.language;
  els.languageSelectDashboard.value = state.language;
}

function applyLanguage() {
  document.documentElement.lang = state.language === "en" ? "en" : "ro";
  document.getElementById("login-language-label").textContent = t("language");
  document.getElementById("dashboard-language-label").textContent = t("language");
  document.getElementById("login-eyebrow").textContent = t("localAdmin");
  document.getElementById("reader-eyebrow").textContent = t("readerEyebrow");
  els.readerIdentityLabel.textContent = t("readerCategory");
  els.readerSummaryTitle.textContent = t("readerSummary");
  document.getElementById("topbar-eyebrow").textContent = t("localHttp");
  document.getElementById("login-title").textContent = t("loginTitle");
  document.getElementById("login-subtitle").textContent = t("loginSubtitle");
  document.getElementById("label-username").textContent = t("username");
  document.getElementById("label-password").textContent = t("password");
  document.getElementById("login-submit").textContent = t("login");
  if (document.getElementById("reader-settings-title")) document.getElementById("reader-settings-title").textContent = t("settingsReader");
  if (document.getElementById("repeat-mode-label")) document.getElementById("repeat-mode-label").textContent = t("repeatModeLabel");
  if (document.getElementById("repeat-mode-help")) document.getElementById("repeat-mode-help").textContent = t("repeatModeHelp");
  if (document.getElementById("save-reader-settings")) document.getElementById("save-reader-settings").textContent = t("saveReaderSettings");
  if (els.repeatModeSelect) {
    const options = els.repeatModeSelect.querySelectorAll("option");
    if (options[0]) options[0].textContent = t("repeatModeIndividual");
    if (options[1]) options[1].textContent = t("repeatModeGrouped");
  }
  document.getElementById("session-label").textContent = t("sessionLabel");
  document.getElementById("status-title").textContent = t("status");
  document.getElementById("logs-title").textContent = t("recentLogs");
  document.getElementById("log-polling-label").textContent = t("logPolling");
  if (document.getElementById("analytes-title")) document.getElementById("analytes-title").textContent = t("navSettings");
  if (els.navSublinks[0]) els.navSublinks[0].textContent = t("settingsReader");
  if (els.navSublinks[1]) els.navSublinks[1].textContent = t("navAnalytes");
  if (els.navSublinks[2]) els.navSublinks[2].textContent = t("settingsDailyDetails");
  if (els.navSublinks[3]) els.navSublinks[3].textContent = t("settingsQc");
  document.getElementById("orders-title").textContent = t("navOrders");
  document.getElementById("qc-summary-title").textContent = t("qcToday");
  document.getElementById("qc-period-label").textContent = t("westgardPeriod");
  document.getElementById("qc-date-from-label").textContent = t("westgardDateFrom");
  document.getElementById("qc-date-to-label").textContent = t("westgardDateTo");
  document.getElementById("new-qc-record").textContent = t("qcAddManual");
  document.getElementById("qc-record-title").textContent = t("qcManualTitle");
  document.getElementById("qc-record-date-label").textContent = t("qcDate");
  document.getElementById("qc-record-analyte-label").textContent = t("qcTargetAnalyte");
  document.getElementById("qc-record-lot-label").textContent = t("lotNo");
  document.getElementById("qc-record-level-label").textContent = t("qcTargetLevel");
  document.getElementById("qc-record-label-label").textContent = t("qcControlLabel");
  document.getElementById("qc-record-qualitative-label").textContent = t("analysisQualitative");
  document.getElementById("qc-record-quantitative-label").textContent = t("analysisQuantitative");
  document.getElementById("qc-record-interpretation-label").textContent = t("interpretation");
  document.getElementById("qc-round-label").textContent = t("qcRound");
  document.getElementById("qc-analyte-filter-label").textContent = t("westgardAnalyte");
  document.getElementById("qc-target-title").textContent = t("qcTargetTitle");
  document.getElementById("qc-target-filter-label").textContent = t("qcTargetAnalyte");
  document.getElementById("qc-target-analyte-label").textContent = t("qcTargetAnalyte");
  document.getElementById("qc-target-level-label").textContent = t("qcTargetLevel");
  document.getElementById("qc-target-lot-label").textContent = t("qcTargetLot");
  document.getElementById("qc-target-mean-label").textContent = t("qcTargetMean");
  document.getElementById("qc-target-sd-label").textContent = t("qcTargetSD");
  document.getElementById("qc-target-1sd-label").textContent = t("qcTargetOneSD");
  document.getElementById("qc-target-2sd-label").textContent = t("qcTargetTwoSD");
  document.getElementById("qc-target-3sd-label").textContent = t("qcTargetThreeSD");
  document.getElementById("qc-target-cv-label").textContent = t("qcTargetCV");
  document.getElementById("save-qc-target").textContent = t("saveQcTarget");
  document.getElementById("delete-qc-target").textContent = t("deleteQcTarget");
  document.getElementById("new-qc-target").textContent = t("new");
  document.getElementById("qc-level-filter-label").textContent = t("qcControlType");
  document.getElementById("qc-details-title").textContent = t("qcDetails");
  document.getElementById("qc-westgard-title").textContent = t("westgardTitle");
  document.getElementById("qc-westgard-period-label").textContent = t("westgardPeriod");
  document.getElementById("qc-westgard-date-from-label").textContent = t("westgardDateFrom");
  document.getElementById("qc-westgard-date-to-label").textContent = t("westgardDateTo");
  els.qcOpenWestgardBtn.textContent = t("westgardTitle");
  document.getElementById("order-details-title").textContent = t("orderDetails");
  if (document.getElementById("daily-details-date-label")) document.getElementById("daily-details-date-label").textContent = t("orderDate");
  if (document.getElementById("daily-details-round-label")) document.getElementById("daily-details-round-label").textContent = t("round");
  if (document.getElementById("daily-details-analyte-label")) document.getElementById("daily-details-analyte-label").textContent = t("navAnalytes");
  document.getElementById("order-date-label").textContent = t("orderDate");
  document.getElementById("round-label").textContent = t("round");
  document.getElementById("orders-select-all-label").textContent = t("selectAll");
  document.getElementById("import-orders").textContent = t("importFile");
  document.getElementById("export-orders").textContent = t("exportFile");
  document.getElementById("worklist-orders").textContent = t("getWorklist");
  document.getElementById("new-round").textContent = t("newRound");
  document.getElementById("editor-title").textContent = t("analyteEditor");
  document.getElementById("help-title").textContent = t("howTo");
  document.getElementById("today-chart-title").textContent = t("todayResults");
  document.getElementById("trend-chart-title").textContent = t("dailyTrend");
  document.getElementById("help-open").textContent = t("openSeparate");
  document.getElementById("save-analyte").textContent = t("save");
  document.getElementById("refresh-analytes").textContent = t("refresh");
  document.getElementById("refresh-qc-targets").textContent = t("refresh");
  els.closeAnalyteModalBtn.textContent = t("close");
  els.closeQCTargetModalBtn.textContent = t("close");
  els.closeQCRecordModalBtn.textContent = t("close");
  els.closeQCWestgardModalBtn.textContent = t("close");
  els.appToastClose.setAttribute("aria-label", t("close"));
  els.closeAnalyteModalBtn.setAttribute("aria-label", t("close"));
  els.closeQCTargetModalBtn.setAttribute("aria-label", t("close"));
  els.closeQCRecordModalBtn.setAttribute("aria-label", t("close"));
  els.closeQCWestgardModalBtn.setAttribute("aria-label", t("close"));
  setImportButtonState(els.importOrdersBtn.classList.contains("is-loading"));
  setAnalyteRefreshButtonState(els.refreshAnalytesBtn.classList.contains("is-loading"));
  renderReaderSidebar();
  document.getElementById("analyte-search").placeholder = t("searchAnalytes");
  if (els.dailyDetailDefinitionSearch) {
    els.dailyDetailDefinitionSearch.placeholder = state.language === "en"
      ? "Search by key or label"
      : "Cauta dupa cheie sau eticheta";
  }
  if (els.dailyDetailsValueSearch) {
    els.dailyDetailsValueSearch.placeholder = state.language === "en"
      ? "Search by variable name"
      : "Cauta dupa nume variabila";
  }
  document.querySelectorAll("[data-i18n]").forEach((node) => {
    const key = dataI18nKey(node.dataset.i18n);
    if (translations[state.language]?.[key] || translations.ro[key]) {
      node.textContent = t(key);
    }
  });
  syncQCWestgardPeriodControls();
  renderTopbarTitle();
  applyBarcodeOverviewLabels();
}

function applyBarcodeOverviewLabels() {
  if (!state.barcodeMode) return;
  const qcTitle = document.getElementById("qc-summary-title");
  const todayTitle = document.getElementById("today-chart-title");
  const trendTitle = document.getElementById("trend-chart-title");
  if (qcTitle) qcTitle.textContent = "Tipariri azi";
  if (todayTitle) todayTitle.textContent = "Status tipariri azi";
  if (trendTitle) trendTitle.textContent = "Evolutie tipariri pe zile";
}

function toggleLogsAccess(canViewLogs) {
  els.logsPanel.hidden = !canViewLogs;
  if (!canViewLogs && state.logPollTimer) {
    clearInterval(state.logPollTimer);
    state.logPollTimer = null;
  }
}

function updateConnectionPills(connections) {
  const wsConnected = !!connections.wisemed_ws_connected;
  const analyzerConnected = !!connections.analyzer_connected;
  els.wisemedwsPill.classList.toggle("connected", wsConnected);
  els.wisemedwsPill.classList.toggle("disconnected", !wsConnected);
  if (state.barcodeMode) {
    els.analyzerPill.hidden = true;
  } else {
    els.analyzerPill.hidden = false;
    els.analyzerPill.classList.toggle("connected", analyzerConnected);
    els.analyzerPill.classList.toggle("disconnected", !analyzerConnected);
  }
  els.wsDot.classList.toggle("offline", !wsConnected);
  els.analyzerDot.classList.toggle("offline", !analyzerConnected);
  els.wisemedwsStatusLabel.textContent = `${t("wisemedws")} · ${wsConnected ? t("connected") : t("disconnected")}`;
  els.analyzerStatusLabel.textContent = `${t("analyzer")} · ${analyzerConnected ? t("connected") : t("disconnected")}`;
}

function inferReaderCategory(readerInfo = {}) {
  const analyzerName = String(readerInfo.analyzer_name || "").toLowerCase();
  const label = String(readerInfo.label || "").toLowerCase();
  const haystack = `${analyzerName} ${label}`;
  if (/(urine|urin|uro)/.test(haystack)) return { key: "readerCategoryUrine", theme: "theme-urine", icon: "U" };
  if (/(hema|cbc|blood|hemat)/.test(haystack)) return { key: "readerCategoryHematology", theme: "theme-hematology", icon: "H" };
  if (/(immun|elisa|clia|eclia)/.test(haystack)) return { key: "readerCategoryImmunology", theme: "theme-immunology", icon: "I" };
  if (/(gas|chromat)/.test(haystack)) return { key: "readerCategoryGas", theme: "theme-gas", icon: "G" };
  if (/(spectro|photomet|colorimet)/.test(haystack)) return { key: "readerCategorySpectro", theme: "theme-spectro", icon: "S" };
  if (/(biochim|biochem|chemistry|chem)/.test(haystack)) return { key: "readerCategoryBiochemistry", theme: "theme-biochemistry", icon: "B" };
  return { key: "readerCategoryGeneric", theme: "theme-biochemistry", icon: "R" };
}

function renderReaderSidebar() {
  const reader = state.readerInfo || {};
  const category = inferReaderCategory(reader);
  els.readerIdentityBadge.className = `reader-identity-badge ${category.theme}`;
  els.readerIdentityIcon.textContent = category.icon;
  els.readerIdentityTitle.textContent = t(category.key);
  els.readerIdentitySubtitle.textContent = reader.analyzer_name || t("readerCategoryGeneric");
  const items = [
    { label: t("medicalUnitLabel"), value: reader.medical_unit_id ?? "-" },
    { label: t("equipmentTypeLabel"), value: reader.equipment_type_id ?? "-" },
    { label: t("equipmentIdLabel"), value: reader.equipment_id ?? "-" },
    { label: t("readerIdLabel"), value: reader.id || "-" },
    { label: t("analyzerNameLabel"), value: reader.analyzer_name || "-" },
  ];
  els.readerSummaryList.innerHTML = items.map((item) => `
    <div class="reader-summary-item">
      <span class="label">${escapeHtml(item.label)}</span>
      <span class="value">${escapeHtml(String(item.value))}</span>
    </div>`).join("");
}

function localizeOrderStatus(status) {
  const normalized = String(status || "").trim().toLowerCase();
  const mapping = {
    received: "statusReceived",
    pending: "statusPending",
    completed: "statusCompleted",
    failed: "statusFailed",
    processing: "statusProcessing",
    imported: "statusImported",
  };
  return t(mapping[normalized] || normalized || "-");
}

function renderDonut(today) {
  const withoutResult = Number(today.without_result || 0);
  const withResult = Number(today.with_result || 0);
  const total = Math.max(withoutResult + withResult, 1);
  const withDeg = Math.round((withResult / total) * 360);
  els.todayDonut.style.background = `conic-gradient(#2f6de1 0deg ${withDeg}deg, #ffb347 ${withDeg}deg 360deg)`;
  if (state.barcodeMode) {
    els.todayLegend.innerHTML = `
      <div class="legend-item"><span class="legend-swatch" style="background:#2f6de1"></span><span>Tipariri OK: ${withResult}</span></div>
      <div class="legend-item"><span class="legend-swatch" style="background:#ffb347"></span><span>Tipariri fail: ${withoutResult}</span></div>`;
    return;
  }
  els.todayLegend.innerHTML = `
    <div class="legend-item"><span class="legend-swatch" style="background:#2f6de1"></span><span>${escapeHtml(t("analysesWithResult"))}: ${withResult}</span></div>
    <div class="legend-item"><span class="legend-swatch" style="background:#ffb347"></span><span>${escapeHtml(t("analysesWithoutResult"))}: ${withoutResult}</span></div>`;
}

function renderQCSummary(summary) {
  if (state.barcodeMode) {
    const cards = [
      { label: "Joburi", value: Number(summary.results || 0) },
      { label: "Etichete", value: Number(summary.numeric_results || 0) },
      { label: "Fail", value: Number(summary.outside_3sd || 0) },
    ];
    els.qcSummary.innerHTML = cards.map((item) => `<div class="metric"><span class="muted small">${escapeHtml(item.label)}</span><strong>${escapeHtml(String(item.value))}</strong></div>`).join("");
    return;
  }
  const cards = [
    { label: t("qcResults"), value: Number(summary.results || 0) },
    { label: t("numericResults"), value: Number(summary.numeric_results || 0) },
    { label: t("outside2sd"), value: Number(summary.outside_2sd || 0) },
    { label: t("outside3sd"), value: Number(summary.outside_3sd || 0) },
  ];
  els.qcSummary.innerHTML = cards.map((item) => `<div class="metric"><span class="muted small">${escapeHtml(item.label)}</span><strong>${escapeHtml(String(item.value))}</strong></div>`).join("");
}

function renderLineChart(series) {
  if (!Array.isArray(series) || series.length === 0) {
    els.lineChart.innerHTML = `<text x="320" y="130" text-anchor="middle" font-size="14" fill="#6d7b73">${escapeHtml(t("noOrders"))}</text>`;
    els.lineLegend.innerHTML = "";
    return;
  }
  const width = 640;
  const height = 260;
  const pad = 24;
  const maxValue = Math.max(1, ...series.flatMap((p) => [p.orders || 0, p.analyses || 0, p.analyses_with_result || 0]));
  const xStep = series.length > 1 ? (width - pad * 2) / (series.length - 1) : 0;
  const toY = (value) => height - pad - ((value / maxValue) * (height - pad * 2));
  const toPath = (key) => series.map((point, index) => `${index === 0 ? "M" : "L"} ${pad + xStep * index} ${toY(point[key] || 0)}`).join(" ");
  const axes = `<line x1="${pad}" y1="${height - pad}" x2="${width - pad}" y2="${height - pad}" stroke="#a8b8ae" stroke-width="1"/><line x1="${pad}" y1="${pad}" x2="${pad}" y2="${height - pad}" stroke="#a8b8ae" stroke-width="1"/>`;
  const labels = series.map((point, index) => `<text x="${pad + xStep * index}" y="${height - 6}" text-anchor="middle" font-size="11" fill="#6d7b73">${escapeHtml((point.day || "").slice(5))}</text>`).join("");
  els.lineChart.innerHTML = `${axes}
    <path d="${toPath("orders")}" fill="none" stroke="#2f6de1" stroke-width="3"/>
    <path d="${toPath("analyses")}" fill="none" stroke="#198a78" stroke-width="3"/>
    <path d="${toPath("analyses_with_result")}" fill="none" stroke="#d85c74" stroke-width="3"/>
    ${labels}`;
  els.lineLegend.innerHTML = `
    <div class="legend-item"><span class="legend-swatch" style="background:#2f6de1"></span><span>${escapeHtml(t("orders"))}</span></div>
    <div class="legend-item"><span class="legend-swatch" style="background:#198a78"></span><span>${escapeHtml(t("analytes"))}</span></div>
    <div class="legend-item"><span class="legend-swatch" style="background:#d85c74"></span><span>${escapeHtml(t("results"))}</span></div>`;
}

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll("\"", "&quot;")
    .replaceAll("'", "&#39;");
}
