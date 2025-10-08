package ocr

// ApplyParsePolicy корректирует confirmation_needed/reason по правилам PROMPT_PARSE.
func ApplyParsePolicy(pr *ParseResult) {
	// Автоподтверждение при всех условиях:
	auto := pr.Confidence >= 0.80 &&
		pr.MeaningChangeRisk <= 0.20 &&
		pr.BracketedSpansCount == 0 &&
		!pr.NeedsRescan

	if auto {
		pr.ConfirmationNeeded = false
		pr.ConfirmationReason = "none"
		return
	}

	// Иначе запрашиваем подтверждение, выставляем приоритетную причину
	pr.ConfirmationNeeded = true
	switch {
	case pr.Confidence < 0.80:
		pr.ConfirmationReason = "low_confidence"
	case pr.BracketedSpansCount >= 1:
		pr.ConfirmationReason = "bracketed_spans_present"
	case pr.MeaningChangeRisk > 0.20:
		pr.ConfirmationReason = "meaning_change_risk_high"
	case pr.NeedsRescan:
		pr.ConfirmationReason = "has_diagrams_or_low_quality"
	default:
		if pr.ConfirmationReason == "" {
			pr.ConfirmationReason = "low_confidence"
		}
	}
}
