package model

type WorklistKind string

const (
	WorklistSimple  WorklistKind = "simple"
	WorklistComplex WorklistKind = "complex"
)

type Worklist struct {
	Kind   WorklistKind `json:"kind"`
	Simple *SimpleList  `json:"simple,omitempty"`
	Round  *RoundList   `json:"round,omitempty"`
}

type SimpleList struct {
	Items []SimplePatientItem `json:"items"`
}

type SimplePatientItem struct {
	PatientID string      `json:"patient_id"`
	Tests     []TestOrder `json:"tests"`
}

type RoundList struct {
	Rounds []WorkRound `json:"rounds"`
}

type WorkRound struct {
	RoundID string `json:"round_id"`
	Racks   []Rack `json:"racks"`
}

type Rack struct {
	RackID     string     `json:"rack_id"`
	Positions  []Position `json:"positions"`
	MaxSlots   int        `json:"max_slots"`
	RackNumber int        `json:"rack_number"`
}

type Position struct {
	Index   int         `json:"index"`
	Patient PatientWork `json:"patient"`
}

type PatientWork struct {
	PatientID string      `json:"patient_id"`
	Tests     []TestOrder `json:"tests"`
}

type TestOrder struct {
	DisplayName string `json:"display_name"`
	Tag         string `json:"tag"`
}
