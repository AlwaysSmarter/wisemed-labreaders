package config

/**
 * WMLRType WiseMedLaboratoryReader type
 */
type WMLRType int

const (
	Undefined WMLRType = iota
	Hematology
	Immunology
	Biochemestry
	Urine
	Coagulation
	VSH
	Microbiology
)

func (wm WMLRType) String() string {
	switch wm {
	case Hematology:
		return "Hematology"
	case Immunology:
		return "Immunology"
	case Biochemestry:
		return "Biochemestry"
	case Urine:
		return "Urine"
	case Coagulation:
		return "Coagulation"
	case VSH:
		return "VSH"
	case Microbiology:
		return "Microbiology"
	default:
		return "Undefined"
	}
}

func (wm WMLRType) Icon() string {
	switch wm {
	case Hematology:
		return "hematology.png"
	case Immunology:
		return "immunology.png"
	case Biochemestry:
		return "biochemestry.png"
	case Urine:
		return "urine.png"
	case Coagulation:
		return "coagulation.png"
	case VSH:
		return "vsh.png"
	case Microbiology:
		return "microbiology.png"
	default:
		return "empty.png"
	}
}
