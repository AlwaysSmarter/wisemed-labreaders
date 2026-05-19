package model

import "time"

type MedicalUnit struct {
	ID   int                    `json:"id"`
	Code string                 `json:"code,omitempty"`
	Name string                 `json:"name"`
	Raw  map[string]interface{} `json:"raw,omitempty"`
}

type AnalyzerEquipmentType struct {
	ID   int                    `json:"id"`
	Name string                 `json:"name"`
	Raw  map[string]interface{} `json:"raw,omitempty"`
}

type Analyte struct {
	ID                int64                  `json:"id"`
	Active            bool                   `json:"active"`
	Tag               string                 `json:"tag"`
	Code              string                 `json:"code"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	ResultType        string                 `json:"result_type"`
	ResultFormatting  string                 `json:"result_formatting"`
	ResultWeighting   float64                `json:"result_weighting"`
	Transformation    []map[string]string    `json:"transformation"`
	ResultMeasureUnit string                 `json:"result_measure_unit"`
	ResultReagentsSet string                 `json:"result_reagents_set"`
	ProtocolOptions   map[string]interface{} `json:"protocol_options,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type DailyDetailDefinition struct {
	ID           int64                  `json:"id"`
	Key          string                 `json:"key"`
	Label        string                 `json:"label"`
	Scope        string                 `json:"scope"`
	FieldType    string                 `json:"field_type"`
	Placeholder  string                 `json:"placeholder"`
	DefaultValue string                 `json:"default_value"`
	Options      []string               `json:"options,omitempty"`
	Required     bool                   `json:"required"`
	Active       bool                   `json:"active"`
	Source       string                 `json:"source"`
	SortOrder    int                    `json:"sort_order"`
	Meta         map[string]interface{} `json:"meta,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type DailyDetailValue struct {
	ID            int64                  `json:"id"`
	DefinitionKey string                 `json:"definition_key"`
	ScopeDate     string                 `json:"scope_date"`
	RoundNo       int                    `json:"round_no"`
	AnalyteTag    string                 `json:"analyte_tag"`
	ValueText     string                 `json:"value_text"`
	Meta          map[string]interface{} `json:"meta,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

type Order struct {
	ID           int64                  `json:"id"`
	RoundNo      int                    `json:"round_no"`
	OrderDate    string                 `json:"order_date"`
	SampleID     string                 `json:"sample_id"`
	FileID       string                 `json:"file_id"`
	PatientID    string                 `json:"patient_id"`
	PatientName  string                 `json:"patient_name"`
	RackNo       int                    `json:"rack_no"`
	RackPosition int                    `json:"rack_position"`
	ListPosition int                    `json:"list_position"`
	SampleNo     int                    `json:"sample_no"`
	Status       string                 `json:"status"`
	SourceFile   string                 `json:"source_file"`
	Meta         map[string]interface{} `json:"meta,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type Round struct {
	ID        int64     `json:"id"`
	OrderDate string    `json:"order_date"`
	RoundNo   int       `json:"round_no"`
	CreatedAt time.Time `json:"created_at"`
}

type OrderAnalysis struct {
	ID                 int64                  `json:"id"`
	OrderID            int64                  `json:"order_id"`
	AnalyteID          int64                  `json:"analyte_id"`
	AnalyteTag         string                 `json:"analyte_tag"`
	AnalyteName        string                 `json:"analyte_name"`
	AnalyteDescription string                 `json:"analyte_description"`
	Status             string                 `json:"status"`
	RequestedAt        time.Time              `json:"requested_at,omitempty"`
	ReceivedAt         time.Time              `json:"received_at,omitempty"`
	DefaultResultID    int64                  `json:"default_result_id"`
	ResultValue        string                 `json:"result_value"`
	RawValue           string                 `json:"raw_value"`
	Interpreted        string                 `json:"interpreted_value"`
	Unit               string                 `json:"unit"`
	SourceFile         string                 `json:"source_file"`
	Flags              map[string]interface{} `json:"flags,omitempty"`
	Meta               map[string]interface{} `json:"meta,omitempty"`
}

type OrderAnalysisResult struct {
	ID              int64                  `json:"id"`
	OrderAnalysisID int64                  `json:"order_analysis_id"`
	ResultValue     string                 `json:"result_value"`
	RawValue        string                 `json:"raw_value"`
	Interpreted     string                 `json:"interpreted_value"`
	Unit            string                 `json:"unit"`
	SourceFile      string                 `json:"source_file"`
	Flags           map[string]interface{} `json:"flags,omitempty"`
	Meta            map[string]interface{} `json:"meta,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
}

type QCRecord struct {
	ID           int64                  `json:"id"`
	RoundNo      int                    `json:"round_no"`
	RunDate      string                 `json:"run_date"`
	ControlLabel string                 `json:"control_label"`
	ControlLevel string                 `json:"control_level"`
	LotNo        string                 `json:"lot_no"`
	DiluentInfo  string                 `json:"diluent_info"`
	FileID       string                 `json:"file_id"`
	Status       string                 `json:"status"`
	SourceFile   string                 `json:"source_file"`
	Meta         map[string]interface{} `json:"meta,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type QCAnalysis struct {
	ID                 int64                  `json:"id"`
	QCRecordID         int64                  `json:"qc_record_id"`
	AnalyteID          int64                  `json:"analyte_id"`
	AnalyteTag         string                 `json:"analyte_tag"`
	AnalyteName        string                 `json:"analyte_name"`
	ControlLevel       string                 `json:"control_level"`
	LotNo              string                 `json:"lot_no"`
	AnalyteDescription string                 `json:"analyte_description"`
	Status             string                 `json:"status"`
	RequestedAt        time.Time              `json:"requested_at,omitempty"`
	ReceivedAt         time.Time              `json:"received_at,omitempty"`
	DefaultResultID    int64                  `json:"default_result_id"`
	ResultValue        string                 `json:"result_value"`
	RawValue           string                 `json:"raw_value"`
	Interpreted        string                 `json:"interpreted_value"`
	NumericValue       *float64               `json:"numeric_value,omitempty"`
	Unit               string                 `json:"unit"`
	SourceFile         string                 `json:"source_file"`
	Flags              map[string]interface{} `json:"flags,omitempty"`
	Meta               map[string]interface{} `json:"meta,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
}

type QCTarget struct {
	ID           int64                  `json:"id"`
	Active       bool                   `json:"active"`
	AnalyteTag   string                 `json:"analyte_tag"`
	AnalyteName  string                 `json:"analyte_name"`
	ControlLevel string                 `json:"control_level"`
	LotNo        string                 `json:"lot_no"`
	ExpiresAt    string                 `json:"expires_at"`
	Unit         string                 `json:"unit"`
	TargetMean   float64                `json:"target_mean"`
	TargetSD     float64                `json:"target_sd"`
	TargetCV     float64                `json:"target_cv"`
	Notes        string                 `json:"notes"`
	Meta         map[string]interface{} `json:"meta,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type EventLog struct {
	ID        int64                  `json:"id"`
	Level     string                 `json:"level"`
	EventType string                 `json:"event_type"`
	Message   string                 `json:"message"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

type ImportedRecord struct {
	SampleID     string                 `json:"sample_id"`
	FileID       string                 `json:"file_id"`
	PatientID    string                 `json:"patient_id"`
	PatientName  string                 `json:"patient_name"`
	AnalyteTag   string                 `json:"analyte_tag"`
	AnalyteName  string                 `json:"analyte_name"`
	ResultValue  string                 `json:"result_value"`
	RawValue     string                 `json:"raw_value"`
	Interpreted  string                 `json:"interpreted_value"`
	Flags        map[string]interface{} `json:"flags,omitempty"`
	Unit         string                 `json:"unit"`
	RackNo       int                    `json:"rack_no"`
	RackPosition int                    `json:"rack_position"`
	ListPosition int                    `json:"list_position"`
	SampleNo     int                    `json:"sample_no"`
	Meta         map[string]interface{} `json:"meta,omitempty"`
}

type OrderBundle struct {
	Order    Order                 `json:"order"`
	Analyses []OrderAnalysisBundle `json:"analyses"`
}

type OrderAnalysisBundle struct {
	Analysis OrderAnalysis         `json:"analysis"`
	Results  []OrderAnalysisResult `json:"results"`
}

type QCRecordBundle struct {
	Record   QCRecord     `json:"record"`
	Analyses []QCAnalysis `json:"analyses"`
}

type DashboardSeriesPoint struct {
	Day                string `json:"day"`
	Orders             int    `json:"orders"`
	Analyses           int    `json:"analyses"`
	AnalysesWithResult int    `json:"analyses_with_result"`
	QCRecords          int    `json:"qc_records"`
	QCResults          int    `json:"qc_results"`
}
