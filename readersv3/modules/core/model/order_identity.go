package model

import "errors"

var ErrOrderIdentityConflict = errors.New("ID-ul cererii a fost modificat sau este deja folosit. Reîncărcați cererea.")
var ErrOrderIdentityInvalid = errors.New("Completați un ID diferit, confirmați exact cu deacord și folosiți o sesiune autentificată.")

type OrderIDChange struct {
	OrderID          int64  `json:"order_id"`
	ExpectedID       string `json:"expected_id"`
	ExpectedRevision int    `json:"expected_revision"`
	NewID            string `json:"new_id"`
	Confirmation     string `json:"confirmation"`
	Reason           string `json:"reason"`
}

// Manually entered IDs are WiseMED file IDs, not analyzer barcodes to normalize.
func (o Order) ManualFileID() string {
	correction, _ := o.Meta["id_correction"].(map[string]interface{})
	value, _ := correction["new_id"].(string)
	return value
}
