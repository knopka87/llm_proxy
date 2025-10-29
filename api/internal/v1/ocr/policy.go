package ocr

import (
	"llm-proxy/api/internal/v1/ocr/types"
)

// ApplyParsePolicy корректирует confirmation_needed/reason по правилам PROMPT_PARSE.
func ApplyParsePolicy(pr *types.ParseResult) {
	// Автоподтверждение при всех условиях:
	auto := pr.Confidence >= 0.80 &&
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
	case pr.NeedsRescan:
		pr.ConfirmationReason = "has_diagrams_or_low_quality"
	default:
		if pr.ConfirmationReason == "" {
			pr.ConfirmationReason = "low_confidence"
		}
	}
}
