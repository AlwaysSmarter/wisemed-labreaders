package wisemedai

import "strings"

// Terminology transcribed from the eight authored ultrasound formulations in
// HitMed's Ecografie de tiroida.docx. Measurements and case findings are excluded.
func thyroidFieldTerms(key string) []string {
	capsule := "capsulă|hiperecogenă|integră|contur regulat|contur boselat"
	diffuse := "ecostructură|parenchim tiroidian|hipoecogenă|hiperecogenă|omogenă|neomogenă|intens neomogenă|microplaje|microplaje cu ecogenități diferite|striuri fibroase|mici striuri fibroase|aspect pseudonodular|aspect fin granular"
	focal := "formațiune nodulară|formațiuni nodulare|nodul|noduli|hipoecogenă|izoecogenă|neomogenă|structură mixtă|bine delimitată|bine delimitate|halou hipoecogen|halou hipoecogen gros|halou hipoecogen subțire parțial|vascularizație perilezională|vascularizație perinodulară|vascularizație intranodulară|vascularizație intra și perinodulară|nevascularizată|nevascularizate|formațiune microchistică|calcificare perinodulară|calcificare în coajă de ou|formațiuni adiacente"
	measurements := "dimensiuni|diametru|centimetri|milimetri"
	var terms string
	switch key {
	case "capsula":
		terms = capsule
	case "lob_drept":
		terms = "lob drept|lob tiroidian drept|" + measurements
	case "lob_stang":
		terms = "lob stâng|lob tiroidian stâng|" + measurements
	case "istm":
		terms = "istm|istm tiroidian|grosime istm|" + measurements
	case "ecostructura_dreapta":
		terms = "lob drept|" + diffuse
	case "ecostructura_stanga":
		terms = "lob stâng|" + diffuse
	case "modificari_istm":
		terms = "istm tiroidian|joncțiune istmico-lobară|" + diffuse
	case "noduli_dreapta":
		terms = "lob drept|" + focal
	case "noduli_stanga":
		terms = "lob stâng|" + focal
	case "noduli_istm":
		terms = "istm|istmico-lobar stâng|istmico-lobar drept|joncțiune istmico-lobară|" + focal
	case "parenchim_normal":
		terms = "parenchim tiroidian|omogen|omogenă"
	case "absenta_leziunilor":
		terms = "fără leziuni focale|fără formațiuni nodulare"
	case "vascularizatie_normala":
		terms = "vascularizație tiroidiană|vascularizație în limite normale"
	case "vascularizatie":
		terms = "vascularizație tiroidiană|Doppler|vascularizație accentuată difuz|vascularizație discret accentuată difuz|hipervascularizație"
	case "absenta_adenopatiilor":
		terms = "fără adenopatie laterocervicală|fără adenopatii laterocervicale"
	case "ganglioni":
		terms = "ganglioni laterocervicali|adenopatie|adenopatii|micropoliadenopatie|micropoliadenopatie laterocervicală bilaterală|caracter nespecific|bilaterală|laterocervical drept|laterocervical stâng"
	}
	if terms == "" {
		return nil
	}
	return strings.Split(terms, "|")
}

func thyroidStructureTerms(id string) []string {
	fields := map[string][]string{
		"tiroida":        {"capsula", "parenchim_normal", "absenta_leziunilor"},
		"lob_drept":      {"lob_drept", "ecostructura_dreapta", "noduli_dreapta"},
		"istm":           {"istm", "modificari_istm", "noduli_istm"},
		"lob_stang":      {"lob_stang", "ecostructura_stanga", "noduli_stanga"},
		"vascularizatie": {"vascularizatie", "vascularizatie_normala"},
		"ganglioni":      {"ganglioni", "absenta_adenopatiilor"},
	}
	out := []string{}
	seen := map[string]bool{}
	for _, field := range fields[id] {
		for _, term := range thyroidFieldTerms(field) {
			if !seen[speechKey(term)] {
				out = append(out, term)
				seen[speechKey(term)] = true
			}
		}
	}
	return out
}

func configureThyroidContext(t *Template) {
	t.AIContext = &TemplateContext{
		Instructions: `Redactează separat capsula și conturul, lobul drept (dimensiuni, ecostructură, leziuni focale), istmul și joncțiunile istmico-lobare, lobul stâng, vascularizația și ganglionii laterocervicali. Păstrează fiecare nodul ca înregistrare distinctă cu localizarea, dimensiunile și descriptorii săi; nu transfera caracteristicile între noduli sau lobi. Nodulii istmico-lobari aparțin câmpului noduli_istm, cu lateralitatea în text. Formațiunea microchistică este o leziune focală a lobului indicat. Aspectul pseudonodular difuz nu dovedește noduli distincți. Păstrează structura mixtă, calcificarea în coajă de ou, haloul gros sau subțire parțial, intens/discret, difuz, bilateral și caracterul nespecific numai dacă sunt dictate. Nu deduce diagnostice sau clasificări. Vascularizația tiroidiană difuză se distinge de cea intra/perinodulară. Măsurătorile din documentul de referință nu sunt valori implicite; păstrează unitățile și semnul >, fără conversii presupuse. „0.3.2 cm” este ambiguu: cere revizuire, nu îl corecta într-o valoare certă. Absența leziunilor sau adenopatiilor și normalitatea sunt consemnate numai dacă sunt documentate explicit. Folosește dicționarele speech_terms ale secțiunilor pentru recunoașterea termenilor și display_value, niciodată ca dovezi.`,
		Terminology:  `Capsulă: hiperecogenă, integră; contur regulat/boselat. Lobi/istm: ecostructură hipoecogenă/hiperecogenă, omogenă/neomogenă/intens neomogenă, microplaje cu ecogenități diferite, mici striuri fibroase, aspect pseudonodular, aspect fin granular. Noduli: hipoecogeni/izoecogeni, structură mixtă, bine delimitați, halou hipoecogen gros sau subțire parțial, vascularizație perilezională, intranodulară/perinodulară, nevascularizați; formațiune microchistică, calcificare perinodulară în coajă de ou, formațiuni adiacente. Vascularizație tiroidiană: accentuată difuz, discret accentuată difuz, în limite normale. Ganglioni: micropoliadenopatie laterocervicală bilaterală cu caracter nespecific; fără adenopatie laterocervicală. „lob sting” = lob stâng; „stg” = stâng; „intra/perinodulară” = intra- și perinodulară; „bien delimitate” = bine delimitate; „neomogenas” = neomogenă, dacă sensul este neechivoc.`,
		Examples: `Exemple numai de terminologie și redactare, fără valori clinice implicite:
„lob sting, hipoecogen, omogen, aspect fin granular” → lob stâng: ecostructură hipoecogenă, omogenă, cu aspect fin granular.
„istmico-lobar stg, formațiune nodulară, intra perinodulară prezentă” → leziune focală istmico-lobară stângă, cu vascularizație intra- și perinodulară prezentă; nu se mută la lobul stâng.
„vascularizație discret accentuată difuz” → vascularizație tiroidiană discret accentuată difuz; nu se aplică automat nodulilor.
„fără adenopatie laterocervicală” → absența adenopatiilor este documentată explicit; lipsa mențiunii nu înseamnă absență.
Doi noduli dictați separat rămân două înregistrări; fiecare păstrează propriile dimensiuni, ecogenitate, halou, calcificări și vascularizație.`,
	}
}
