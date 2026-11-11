package wisemedai

import (
	"context"
	"encoding/json"
	"html"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type catalogSpec struct {
	Slug, Specialty, Name string
	Fields                []string
}

// Field syntax: key|label|type|unit|min|max. Clinical text is intentionally blank.
var specialtyCatalog = []catalogSpec{
	{"neurologie", "Neurologie", "Consultație neurologie", []string{"cefalee|Cefalee și caracteristici", "constienta|Starea de conștiență", "nervi_cranieni|Nervi cranieni", "forta_musculara|Forță musculară", "sensibilitate|Sensibilitate", "reflexe|Reflexe osteotendinoase", "coordonare|Coordonare și mers", "episoade|Episoade neurologice descrise"}},
	{"cardiologie", "Cardiologie", "Consultație cardiologie extinsă", []string{"durere_toracica|Durere toracică descrisă", "dispnee|Dispnee descrisă", "ta|Tensiune arterială|string|mmHg", "puls|Puls|number|bpm|20|250", "auscultatie_cardiaca|Auscultație cardiacă", "ekg|EKG efectuat și constatări", "ecocardiografie|Ecocardiografie efectuată și constatări", "edeme|Edeme descrise"}},
	{"medicina_interna", "Medicină internă", "Consultație medicină internă", []string{"stare_generala|Stare generală", "temperatura|Temperatură|number|°C|30|45", "ta|Tensiune arterială|string|mmHg", "puls|Puls|number|bpm|20|250", "examen_cardiopulmonar|Examen cardiopulmonar", "examen_abdominal|Examen abdominal", "status_hidratare|Status de hidratare", "comorbiditati|Comorbidități documentate"}},
	{"medicina_familie", "Medicină de familie", "Consultație medicină de familie", []string{"motiv_prezentare|Motivul prezentării", "istoric_personal|Istoric personal relevant", "alergii|Alergii documentate", "medicatie_curenta|Medicație curentă declarată", "ta|Tensiune arterială|string|mmHg", "greutate|Greutate|number|kg|0.2|500", "inaltime|Înălțime|number|cm|20|250", "preventie|Consiliere preventivă efectuată"}},
	{"pediatrie", "Pediatrie", "Consultație pediatrie", []string{"varsta_luni|Vârstă menționată|number|luni|0|216", "insotitor|Însoțitor și sursa anamnezei", "antecedente_perinatale|Antecedente perinatale", "alimentatie|Alimentație", "dezvoltare|Dezvoltare descrisă", "vaccinari|Vaccinări documentate", "greutate|Greutate|number|kg|0.2|250", "temperatura|Temperatură|number|°C|30|45", "hidratare|Hidratare descrisă"}},
	{"geriatrie", "Geriatrie și gerontologie", "Evaluare geriatrică", []string{"autonomie|Autonomie funcțională descrisă", "cognitie|Status cognitiv examinat", "mobilitate|Mobilitate și căderi declarate", "nutritie|Status nutrițional", "polimedicatie|Medicație declarată", "suport_social|Suport social autorizat", "fragilitate|Evaluare de fragilitate efectuată"}},
	{"psihiatrie", "Psihiatrie", "Consultație psihiatrie", []string{"dispozitie|Dispoziție descrisă", "afect|Afect observat", "gandire|Gândire și conținut", "perceptie|Percepție", "somn|Somn declarat", "comportament|Comportament observat", "evaluare_risc|Evaluarea riscului efectuată și documentată", "consimtamant|Consimțământ documentat"}},
	{"psihiatrie_pediatrica", "Psihiatrie pediatrică", "Consultație psihiatrie pediatrică", []string{"insotitor|Însoțitor și sursa informațiilor", "dezvoltare|Dezvoltare psihomotorie", "context_scolar|Context școlar declarat", "comportament|Comportament observat", "dispozitie|Dispoziție", "somn|Somn declarat", "evaluare_risc|Evaluare de risc efectuată", "consimtamant|Consimțământ reprezentant documentat"}},
	{"endocrinologie", "Endocrinologie", "Consultație endocrinologie", []string{"simptome_endocrine|Simptome endocrine descrise", "examen_tiroida|Examen tiroidian", "greutate|Greutate|number|kg|0.2|500", "inaltime|Înălțime|number|cm|20|250", "status_pubertar|Status pubertar examinat", "ecografie_tiroida|Ecografie tiroidiană efectuată", "rezultate_hormonale|Rezultate hormonale documentate"}},
	{"diabetologie", "Diabet zaharat, nutriție și boli metabolice", "Consultație diabetologie și nutriție", []string{"tip_diabet_documentat|Tip de diabet documentat", "glicemie|Glicemie documentată|number|mg/dL|10|1500", "hba1c|HbA1c documentată|number|%|2|25", "episoade_hipoglicemie|Episoade de hipoglicemie declarate", "automonitorizare|Automonitorizare declarată", "examen_picior|Examenul piciorului efectuat", "alimentatie|Alimentație descrisă", "greutate|Greutate|number|kg|0.2|500"}},
	{"pneumologie", "Pneumologie", "Consultație pneumologie", []string{"tuse|Tuse descrisă", "dispnee|Dispnee descrisă", "fumat|Expunere tabagică declarată", "spo2|Saturație periferică măsurată|number|%|30|100", "frecventa_respiratorie|Frecvență respiratorie|number|/min|3|100", "auscultatie_pulmonara|Auscultație pulmonară", "spirometrie|Spirometrie efectuată și rezultate", "imagistica_toracica|Imagistică toracică documentată"}},
	{"gastroenterologie", "Gastroenterologie", "Consultație gastroenterologie", []string{"durere_abdominala|Durere abdominală descrisă", "tranzit|Tranzit intestinal declarat", "simptome_digestive|Simptome digestive", "examen_abdominal|Examen abdominal", "examen_hepatic|Examen hepatic", "endoscopie|Endoscopie efectuată și rezultate", "ecografie|Ecografie documentată", "analize_documentate|Analize disponibile"}},
	{"nefrologie", "Nefrologie", "Consultație nefrologie", []string{"diureza|Diureză declarată sau măsurată", "edeme|Edeme examinate", "ta|Tensiune arterială|string|mmHg", "creatinina|Creatinină documentată|number|mg/dL|0.1|40", "egfr_documentat|eGFR raportat|number|mL/min/1.73m²|0|250", "sumar_urina|Sumar de urină documentat", "ecografie_renala|Ecografie renală efectuată", "dializa_context|Context de dializă documentat"}},
	{"urologie", "Urologie", "Consultație urologie", []string{"simptome_urinare|Simptome urinare", "hematurie|Hematurie descrisă", "examen_urologic|Examen urologic", "examen_prostata|Examen prostatic efectuat", "ecografie_urologica|Ecografie efectuată", "reziduu_postmictional|Reziduu postmicțional|number|mL|0|3000", "psa|PSA documentat|number|ng/mL|0|10000"}},
	{"obstetrica", "Obstetrică-ginecologie", "Consultație obstetrică", []string{"varsta_gestationala|Vârstă gestațională declarată", "antecedente_obstetricale|Antecedente obstetricale", "simptome_sarcina|Simptome declarate", "ta|Tensiune arterială|string|mmHg", "miscari_fetale|Mișcări fetale declarate", "bcf|Frecvență cardiacă fetală măsurată|number|bpm|40|250", "examen_obstetrical|Examen obstetrical efectuat", "ecografie_obstetricala|Ecografie efectuată și măsurători"}},
	{"ginecologie", "Obstetrică-ginecologie", "Consultație ginecologie", []string{"istoric_menstrual|Istoric menstrual", "antecedente_ginecologice|Antecedente ginecologice", "simptome_ginecologice|Simptome descrise", "examen_ginecologic|Examen ginecologic efectuat", "examen_col|Examen de col efectuat", "ecografie_pelvina|Ecografie pelvină efectuată", "citologie_documentata|Citologie documentată"}},
	{"dermatologie", "Dermatovenerologie", "Consultație dermatologie", []string{"debut_evolutie|Debut și evoluție", "localizare|Localizarea leziunilor", "leziuni|Leziuni cutanate descrise", "simptome_cutante|Simptome cutanate declarate", "dermatoscopie|Dermatoscopie efectuată", "mucoase|Examenul mucoaselor", "expuneri|Expuneri declarate", "fotodocumentare|Fotodocumentare cu consimțământ"}},
	{"orl", "Otorinolaringologie", "Consultație ORL", []string{"otoscopie|Otoscopie efectuată", "rinologie|Examen rinologic", "orofaringe|Examen orofaringian", "laringoscopie|Laringoscopie efectuată", "auz|Evaluare auditivă efectuată", "echilibru|Examen vestibular efectuat", "examen_cervical|Examen cervical"}},
	{"oftalmologie", "Oftalmologie", "Consultație oftalmologie", []string{"acuitate_od|Acuitate vizuală ochi drept", "acuitate_os|Acuitate vizuală ochi stâng", "refractie|Refracție măsurată", "tio_od|Presiune intraoculară OD|number|mmHg|1|80", "tio_os|Presiune intraoculară OS|number|mmHg|1|80", "segment_anterior|Examen segment anterior", "fund_ochi|Fund de ochi examinat", "oct|OCT efectuat și rezultate"}},
	{"reumatologie", "Reumatologie", "Consultație reumatologie", []string{"durere_articulara|Durere articulară", "redoare|Redoare matinală declarată", "articulatii|Articulații examinate", "manifestari_extraarticulare|Manifestări extraarticulare", "mobilitate|Mobilitate examinată", "scor_documentat|Scor validat efectuat și documentat", "imagistica_documentata|Imagistică documentată", "biologie_documentata|Rezultate biologice disponibile"}},
	{"ortopedie", "Ortopedie și traumatologie", "Consultație ortopedie-traumatologie", []string{"mecanism|Mecanism traumatic declarat", "segment_lateralitate|Segment și lateralitate", "inspectie|Inspecție", "palpare|Palpare", "mobilitate|Mobilitate examinată", "status_neurovascular|Status neurovascular examinat", "imagistica|Imagistică documentată", "imobilizare_efectuata|Imobilizare efectuată și documentată"}},
	{"chirurgie_generala", "Chirurgie generală", "Consultație chirurgie generală", []string{"istoric_chirurgical|Antecedente chirurgicale", "durere|Durere descrisă", "examen_abdominal|Examen abdominal", "examen_local|Examen local", "plaga|Plagă examinată", "imagistica|Imagistică documentată", "proceduri_efectuate|Proceduri efectiv efectuate", "indicatie_documentata|Indicație documentată de medic"}},
	{"chirurgie_vasculara", "Chirurgie vasculară", "Consultație chirurgie vasculară", []string{"simptome_vasculare|Simptome vasculare", "pulsuri|Pulsuri periferice examinate", "tegumente|Tegumente examinate", "edeme|Edeme descrise", "ulceratii|Ulcerații examinate", "indice_glezna_brat|Indice gleznă-braț măsurat|number||0|3", "doppler|Doppler efectuat și rezultate"}},
	{"chirurgie_cardiovasculara", "Chirurgie cardiovasculară", "Consultație chirurgie cardiovasculară", []string{"context_cardiovascular|Context cardiovascular documentat", "toleranta_efort|Toleranță la efort declarată", "examen_cardiovascular|Examen cardiovascular", "ecocardiografie|Ecocardiografie documentată", "coronarografie|Coronarografie documentată", "status_postoperator|Status postoperator examinat", "indicatie_documentata|Indicație explicit documentată"}},
	{"chirurgie_toracica", "Chirurgie toracică", "Consultație chirurgie toracică", []string{"simptome_toracice|Simptome toracice", "examen_toracic|Examen toracic", "imagistica|Imagistică documentată", "explorari_functionale|Explorări funcționale documentate", "drenaj|Drenaj examinat și măsurat", "status_postoperator|Status postoperator", "indicatie_documentata|Indicație documentată"}},
	{"neurochirurgie", "Neurochirurgie", "Consultație neurochirurgie", []string{"simptome_neurologice|Simptome neurologice", "constienta|Stare de conștiență", "deficit_motor|Deficit motor examinat", "sensibilitate|Sensibilitate examinată", "examen_rahis|Examenul rahisului", "imagistica|CT/RMN documentat", "status_postoperator|Status postoperator", "indicatie_documentata|Indicație documentată"}},
	{"chirurgie_pediatrica", "Chirurgie pediatrică", "Consultație chirurgie pediatrică", []string{"insotitor|Însoțitor și anamneză", "antecedente|Antecedente relevante", "examen_local|Examen local", "examen_abdominal|Examen abdominal", "imagistica|Imagistică documentată", "proceduri_efectuate|Proceduri efectuate", "consimtamant|Consimțământ reprezentant documentat"}},
	{"chirurgie_plastica", "Chirurgie plastică, estetică și microchirurgie reconstructivă", "Consultație chirurgie plastică", []string{"leziune_defect|Leziune sau defect descris", "localizare|Localizare și lateralitate", "dimensiuni|Dimensiuni măsurate", "status_neurovascular|Status neurovascular", "cicatrici|Cicatrici examinate", "fotodocumentare|Fotodocumentare autorizată", "proceduri_efectuate|Proceduri efectuate", "consimtamant|Consimțământ documentat"}},
	{"oncologie", "Oncologie medicală", "Consultație oncologie medicală", []string{"diagnostic_confirmat|Diagnostic oncologic documentat", "histopatologie|Histopatologie documentată", "stadializare_documentata|Stadializare documentată", "status_performanta|Status de performanță evaluat", "tratament_in_curs|Tratament documentat în curs", "toxicitate|Toxicitate descrisă", "imagistica|Imagistică disponibilă", "discutie_multidisciplinara|Concluzii multidisciplinare documentate"}},
	{"hematologie", "Hematologie", "Consultație hematologie", []string{"simptome_hematologice|Simptome declarate", "adenopatii|Adenopatii examinate", "hepatosplenomegalie|Examen hepato-splenic", "hemograma|Hemogramă documentată", "frotiu|Frotiu documentat", "explorari_medulare|Explorări medulare efectuate", "hemostaza|Rezultate de hemostază", "tratament_documentat|Tratament documentat"}},
	{"boli_infectioase", "Boli infecțioase", "Consultație boli infecțioase", []string{"context_epidemiologic|Context epidemiologic declarat", "debut|Debut și evoluție", "temperatura|Temperatură|number|°C|30|45", "examen_infectios|Examen clinic", "microbiologie|Rezultate microbiologice documentate", "expuneri|Expuneri relevante", "vaccinari|Vaccinări documentate", "masuri_documentate|Măsuri indicate explicit de medic"}},
	{"alergologie", "Alergologie și imunologie clinică", "Consultație alergologie-imunologie", []string{"alergeni_suspectati|Alergeni declarați", "reactii|Reacții descrise", "context_reactie|Contextul reacțiilor", "examen_cutaneo_mucos|Examen cutaneo-mucos", "examen_respirator|Examen respirator", "teste_efectuate|Teste efectuate și rezultate", "antecedente_imunologice|Antecedente imunologice documentate"}},
	{"recuperare", "Medicină fizică și de reabilitare", "Consultație medicină fizică și reabilitare", []string{"limitari_functionale|Limitări funcționale", "durere|Durere declarată", "mobilitate|Mobilitate măsurată", "forta|Forță musculară examinată", "mers|Mers examinat", "autonomie|Autonomie descrisă", "obiective_agreate|Obiective agreate cu pacientul", "proceduri_efectuate|Proceduri efectuate"}},
	{"medicina_muncii", "Medicina muncii", "Consultație medicina muncii", []string{"ocupatie|Ocupație declarată", "expuneri_profesionale|Expuneri profesionale documentate", "istoric_profesional|Istoric profesional", "simptome_profesionale|Simptome declarate", "examen_specific|Examen clinic specific", "investigatii_documentate|Investigații documentate", "aviz_documentat|Aviz explicit formulat de medic"}},
	{"medicina_sportiva", "Medicină sportivă", "Consultație medicină sportivă", []string{"sport|Sport și nivel practicat", "istoric_efort|Istoric de efort", "antecedente_sportive|Antecedente relevante", "examen_locomotor|Examen locomotor", "examen_cardiovascular|Examen cardiovascular", "teste_efort|Teste de efort efectuate", "aviz_documentat|Aviz documentat de medic"}},
	{"urgenta", "Medicină de urgență", "Evaluare medicină de urgență", []string{"motiv_urgenta|Motivul prezentării", "debut|Debut și mecanism declarat", "triaj_documentat|Triaj documentat de personal", "evaluare_abcde|Evaluare ABCDE efectuată", "ta|Tensiune arterială|string|mmHg", "puls|Puls|number|bpm|20|250", "spo2|SpO2|number|%|30|100", "proceduri_efectuate|Proceduri efectiv efectuate", "dispozitie_documentata|Dispoziție decisă de medic"}},
	{"ati", "Anestezie și terapie intensivă", "Vizită terapie intensivă", []string{"context_internare|Context de internare documentat", "status_neurologic|Status neurologic examinat", "status_respirator|Status respirator", "suport_ventilator|Suport ventilator documentat", "hemodinamica|Hemodinamică măsurată", "diureza|Diureză măsurată", "bilant_hidric|Bilanț hidric documentat", "dispozitive|Dispozitive examinate", "proceduri_efectuate|Proceduri efectuate"}},
	{"anestezie", "Anestezie și terapie intensivă", "Evaluare preanestezică", []string{"procedura_planificata|Procedură planificată documentată", "antecedente_anestezice|Antecedente anestezice", "alergii|Alergii declarate", "medicatie_curenta|Medicație declarată", "cale_aeriana|Examen cale aeriană", "status_cardiorespirator|Status cardiorespirator", "clasificare_documentata|Clasificare explicit documentată", "consimtamant|Consimțământ documentat"}},
	{"neonatologie", "Neonatologie", "Evaluare neonatologie", []string{"varsta_gestationala|Vârstă gestațională documentată", "nastere|Contextul nașterii", "greutate|Greutate|number|g|200|10000", "alimentatie|Alimentație", "adaptare|Adaptare neonatală documentată", "examen_neonatal|Examen neonatal efectuat", "icter|Icter examinat", "screening_documentat|Screening efectuat și documentat"}},
	{"radiologie", "Radiologie și imagistică medicală", "Raport imagistic general", []string{"indicatie_documentata|Indicație documentată", "modalitate|Modalitate efectuată", "regiune|Regiune și lateralitate", "tehnica|Tehnică efectuată", "contrast|Substanță de contrast efectiv administrată", "comparatie|Examinări comparative disponibile", "constatari_imagistice|Constatări observate", "leziuni|Leziuni descrise", "concluzie_documentata|Concluzie explicit formulată"}},
	{"ecografie_tiroida", "Radiologie și imagistică medicală", "Ecografie tiroidiană", nil},
	{"ecocardiografie", "Cardiologie", "Raport ecocardiografie", nil},
	{"stomatologie", "Medicină dentară", "Consultație stomatologie", []string{"durere_dentara|Durere dentară", "status_dentar|Status dentar examinat", "status_parodontal|Status parodontal examinat", "mucoasa_orala|Mucoasă orală examinată", "ocluzie|Ocluzie examinată", "imagistica_dentara|Imagistică documentată", "proceduri_efectuate|Proceduri efectiv efectuate", "consimtamant|Consimțământ documentat"}},
	{"chirurgie_omf", "Chirurgie orală și maxilo-facială", "Consultație chirurgie OMF", []string{"examen_facial|Examen facial", "examen_endobucal|Examen endobucal", "ocluzie|Ocluzie", "sensibilitate|Sensibilitate examinată", "imagistica|Imagistică documentată", "leziuni|Leziuni descrise", "proceduri_efectuate|Proceduri efectuate", "consimtamant|Consimțământ documentat"}},
	{"ingrijiri_paliative", "Îngrijiri paliative", "Evaluare îngrijiri paliative", []string{"simptome_prioritare|Simptome prioritare declarate", "durere|Durere evaluată și documentată", "dispnee|Dispnee descrisă", "status_functional|Status funcțional", "nevoi_psihosociale|Nevoi psihosociale exprimate", "preferinte_pacient|Preferințe exprimate de pacient", "discutii_documentate|Discuții efectuate", "plan_agreat|Plan explicit agreat și documentat"}},
	{"radioterapie", "Radioterapie", "Consultație radioterapie", []string{"diagnostic_documentat|Diagnostic oncologic documentat", "context_tratament|Context de tratament", "iradiere_documentata|Iradiere efectiv administrată documentată", "doza_documentata|Doză documentată", "toxicitate|Toxicitate descrisă", "status_local|Status local examinat", "imagistica|Imagistică documentată"}},
	{"anatomie_patologica", "Anatomie patologică", "Raport anatomie patologică", []string{"specimen|Specimen primit documentat", "localizare|Localizare documentată", "macroscopie|Descriere macroscopică", "microscopie|Descriere microscopică", "teste_efectuate|Teste efectuate și rezultate", "diagnostic_documentat|Diagnostic explicit formulat de anatomopatolog", "limitari|Limitări ale materialului"}},
	{"medicina_laborator", "Medicină de laborator", "Notă interpretativă laborator", []string{"specimen|Specimen documentat", "recoltare|Context de recoltare documentat", "calitate_proba|Calitatea probei evaluată", "metode|Metode efectuate", "rezultate_documentate|Rezultate măsurate documentate", "intervale_referinta|Intervale comunicate de laborator", "comentariu_documentat|Comentariu explicit formulat de medic", "limitari|Limitări analitice"}},
	{"medicina_nucleara", "Medicină nucleară", "Raport medicină nucleară", []string{"indicatie_documentata|Indicație documentată", "radiofarmaceutic|Radiofarmaceutic efectiv administrat", "activitate_documentata|Activitate administrată documentată", "tehnica|Tehnică efectuată", "constatari|Constatări observate", "comparatie|Comparație cu examinări disponibile", "concluzie_documentata|Concluzie explicit formulată"}},
}

func catalogTemplate(spec catalogSpec) Template {
	if spec.Slug == "ecografie_tiroida" {
		return thyroidUltrasoundTemplate(spec)
	}
	if spec.Slug == "ecocardiografie" {
		return echocardiographyTemplate(spec)
	}
	t := Template{Name: spec.Name, Specialty: spec.Specialty, Sections: []Section{{ID: "istoric", Label: "Anamneză și context"}, {ID: "examen", Label: "Examinare și investigații efectuate"}, {ID: "sinteza", Label: "Sinteză documentată și propuneri"}}, Rules: []Rule{}, Fields: []Field{}, Fragments: map[string]string{"normal": "<p>Examinare efectuată: [descrieți constatările normale observate și documentate].</p>", "patologic": "<p>Constatare patologică observată: [localizare, lateralitate, aspect, măsurători și sursă].</p>"}, SemanticMapping: map[string]string{"anamneza": "encounter.history", "examen_clinic": "encounter.examination", "concluzii_documentate": "encounter.assessment_documented", "plan_documentat": "encounter.plan_proposed"}}
	t.Fields = append(t.Fields, Field{Key: "anamneza", Label: "Anamneză și motivul prezentării", Type: "string", Required: true, Section: "istoric", Aliases: []string{"anamneză", "anamneza"}, SemanticTag: "encounter.history"}, Field{Key: "examen_clinic", Label: "Examen clinic efectuat", Type: "string", Required: true, Section: "examen", Aliases: []string{"examen clinic"}, SemanticTag: "encounter.examination"})
	var doc strings.Builder
	doc.WriteString("<h2>" + html.EscapeString(spec.Name) + "</h2><p>Referință pacient: {{patient_ref}} · Medic: {{doctor_name}}</p><section data-section=\"istoric\"><h3>Anamneză</h3><p>{{anamneza}}</p></section><section data-section=\"examen\"><h3>Examinare</h3><p>{{examen_clinic}}</p>")
	for _, raw := range spec.Fields {
		parts := strings.Split(raw, "|")
		f := Field{Key: parts[0], Label: parts[1], Type: "string", Section: "examen", Aliases: []string{parts[1]}, SemanticTag: ""}
		if len(parts) > 2 && parts[2] != "" {
			f.Type = parts[2]
		}
		if len(parts) > 3 {
			f.Unit = parts[3]
		}
		if len(parts) > 4 {
			f.Min = parseNumber(parts[4])
		}
		if len(parts) > 5 {
			f.Max = parseNumber(parts[5])
		}
		f.Repeatable = f.Key == "leziuni" || f.Key == "noduli" || f.Key == "reactii"
		t.Fields = append(t.Fields, f)
		doc.WriteString("<p><strong>" + html.EscapeString(f.Label) + ":</strong> {{" + f.Key + "}}")
		if f.Unit != "" {
			doc.WriteString(" " + html.EscapeString(f.Unit))
		}
		doc.WriteString("</p>")
	}
	t.Fields = append(t.Fields, Field{Key: "concluzii_documentate", Label: "Concluzii formulate de medic", Type: "string", Section: "sinteza", SemanticTag: "encounter.assessment_documented"}, Field{Key: "plan_documentat", Label: "Plan documentat — propunere pentru revizuire", Type: "string", Section: "sinteza", SemanticTag: "encounter.plan_proposed"})
	doc.WriteString("</section><section data-section=\"sinteza\"><h3>Sinteză</h3><p>{{concluzii_documentate}}</p><h3>Plan propus pentru revizuire</h3><p>{{plan_documentat}}</p></section>")
	t.HTML = doc.String()
	return adaptCatalogTemplate(t)
}
func parseNumber(s string) *float64 {
	var v float64
	if json.Unmarshal([]byte(s), &v) != nil {
		return nil
	}
	return &v
}
func (m *Module) ensureCatalog(ctx context.Context) error {
	rows, e := m.db.Query(ctx, `SELECT id FROM wai_organizations`)
	if e != nil {
		return e
	}
	tenants := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		tenants = append(tenants, id)
	}
	rows.Close()
	for _, tenant := range tenants {
		tx, e := m.db.Begin(ctx)
		if e != nil {
			return e
		}
		if e = m.seedCatalog(ctx, tx, tenant); e != nil {
			tx.Rollback(ctx)
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
	}
	return nil
}
func (m *Module) seedCatalog(ctx context.Context, tx pgx.Tx, tenant string) error {
	if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "wai-catalog:"+tenant); e != nil {
		return e
	}
	for _, spec := range specialtyCatalog {
		var done bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wai_seed_catalog WHERE tenant_id=$1 AND slug=$2)`, tenant, spec.Slug).Scan(&done); e != nil {
			return e
		}
		if done {
			continue
		}
		t := catalogTemplate(spec)
		if e := m.saveCatalogTemplate(ctx, tx, tenant, &t); e != nil {
			return e
		}
		sid := uuid.NewSHA1(uuid.NameSpaceURL, []byte("wai-specialty:"+tenant+":"+spec.Specialty)).String()
		if _, e := tx.Exec(ctx, `INSERT INTO wai_specialties(id,tenant_id,name)VALUES($1,$2,$3) ON CONFLICT(id)DO NOTHING`, sid, tenant, spec.Specialty); e != nil {
			return e
		}
		packID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("wai-pack:"+tenant+":"+spec.Specialty)).String()
		b, _ := json.Marshal(map[string]any{"catalog_version": "2026.11", "synthetic": true, "contexts": []string{"ambulatoriu", "internare"}, "template_ids": []string{t.ID}, "purpose": "documentare clinică; fără valori normale implicite"})
		if _, e := tx.Exec(ctx, `INSERT INTO wai_specialty_packs(id,tenant_id,specialty_id,name,payload)VALUES($1,$2,$3,$4,$5) ON CONFLICT(id) DO UPDATE SET payload=jsonb_set(wai_specialty_packs.payload,'{template_ids}',coalesce(wai_specialty_packs.payload->'template_ids','[]'::jsonb)||(excluded.payload->'template_ids'))`, packID, tenant, sid, spec.Specialty, b); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO wai_seed_catalog(tenant_id,slug,template_id)VALUES($1,$2,$3)`, tenant, spec.Slug, t.ID); e != nil {
			return e
		}
	}
	return m.upgradeCatalogModel(ctx, tx, tenant)
}

// Catalog reference context follows the same encrypted storage as editor publications.
func (m *Module) saveCatalogTemplate(ctx context.Context, tx pgx.Tx, tenant string, t *Template) error {
	if err := saveTemplate(ctx, tx, tenant, t); err != nil {
		return err
	}
	if t.AIContext == nil {
		return nil
	}
	raw, err := m.box.seal(t.AIContext, tenant+":"+t.VersionID+":context")
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO wai_template_contexts(template_version,tenant_id,created_by,payload)
 SELECT $1,$2,user_id,$3 FROM wai_memberships WHERE tenant_id=$2 ORDER BY (role='admin') DESC,user_id LIMIT 1`, t.VersionID, tenant, raw)
	return err
}
