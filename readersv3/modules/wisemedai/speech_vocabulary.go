package wisemedai

// Versioned template terms are defaults; an explicit saved status takes precedence.
func mergeTemplateSpeechTerms(base []string, saved []speechTerm) []speechTerm {
	out := append([]speechTerm(nil), saved...)
	seen := map[string]bool{}
	for _, term := range saved {
		seen[speechKey(term.Text)] = true
	}
	for _, text := range base {
		if seen[speechKey(text)] {
			continue
		}
		seen[speechKey(text)] = true
		out = append(out, speechTerm{Text: text, Status: "approved", Source: "template", Key: digest(speechKey(text))})
	}
	return out
}

// The phone and OpenAI use the same approved, bounded dictionary for each section.
func activeSpeechTerms(entries []speechTerm) []string {
	out := []string{}
	budget := 0
	seen := map[string]bool{}
	for _, entry := range entries {
		text := normalizeSpeech(entry.Text)
		key := speechKey(text)
		if entry.Status != "approved" || seen[key] || !safeSpeechTerm(text) || len(out) >= 64 || budget+len(text)+1 > activeSpeechBudget {
			continue
		}
		seen[key] = true
		out = append(out, text)
		budget += len(text) + 1
	}
	return out
}
