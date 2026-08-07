package types

import "testing"

func TestParseResponseValidateItems_InvalidatesContradictoryAnswer(t *testing.T) {
	t.Parallel()
	response := ParseResponse{
		Task: ParseTask{Quality: ParseTaskQuality{}},
		Items: []ParseItem{{
			PedKeys: PedKeys{TaskType: "expressions"},
			SolutionInternal: SolutionInternal{
				SolutionSteps: []string{"Последнее действие: 27444 + 32646 = 60090 руб."},
				FinalAnswer:   "59844 руб.",
			},
		}},
	}

	if got := response.ValidateItems(); got != 1 {
		t.Fatalf("ValidateItems()=%d, want 1", got)
	}
	if !response.Items[0].ItemQuality.UnsafeToFinalizeAnswer {
		t.Fatal("contradictory answer was not marked unsafe")
	}
	if response.Items[0].SolutionInternal.FinalAnswer != nil {
		t.Fatal("contradictory final_answer was not cleared")
	}
	if response.Items[0].PedKeys.TaskType != TaskTypeArithmeticFluency {
		t.Fatalf("task type was not normalized: %q", response.Items[0].PedKeys.TaskType)
	}
}

func TestParseResponseValidateItems_AcceptsAnswerWithUnits(t *testing.T) {
	t.Parallel()
	response := ParseResponse{Items: []ParseItem{{
		SolutionInternal: SolutionInternal{
			SolutionSteps: []string{"Последнее действие: 27444 + 32646 = 60090 руб."},
			FinalAnswer:   "60090 рублей",
		},
	}}}
	if got := response.ValidateItems(); got != 0 {
		t.Fatalf("ValidateItems()=%d, want 0", got)
	}
}

func TestParseResponseValidateItems_UsesStructuredVerification(t *testing.T) {
	t.Parallel()
	method := "full_recalculation"
	response := ParseResponse{Items: []ParseItem{{
		SolutionInternal: SolutionInternal{
			SolutionSteps: []string{"Вычисления записаны словами без знака равенства"},
			FinalAnswer:   40,
			Verification: &SolutionVerification{
				Method: &method, DerivedAnswer: 35, Passed: true,
			},
		},
	}}}
	if got := response.ValidateItems(); got != 1 {
		t.Fatalf("ValidateItems()=%d, want 1", got)
	}
	if response.Items[0].SolutionInternal.FinalAnswer != nil {
		t.Fatal("final_answer contradicting structured verification was not cleared")
	}
}

func TestHintResponseValidateAgainstRequest_RejectsPartialCoverage(t *testing.T) {
	t.Parallel()
	request := hintValidationRequest()
	response := validHintResponse()
	response.Items[0].PlanCoverage.PlanStepsCovered = 2
	if err := response.ValidateAgainstRequest(request); err == nil {
		t.Fatal("partial plan coverage was accepted")
	}
}

func TestHintResponseValidateAgainstRequest_AcceptsCompleteCoverage(t *testing.T) {
	t.Parallel()
	if err := validHintResponse().ValidateAgainstRequest(hintValidationRequest()); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}
}

func TestCheckResponseValidateSemantics_RequiresVisualEvidence(t *testing.T) {
	t.Parallel()
	request := CheckRequest{TaskStruct: TaskStructCheck{VisualFacts: []VisualFact{{Kind: "venn_object"}}}}
	response := validCheckResponse()
	if err := response.ValidateSemantics(request); err == nil {
		t.Fatal("visual response without evidence was accepted")
	}
	response.Debug.VisualEvidence = []CheckVisualEvidence{{
		ObjectID: "point_1", Observed: "square,triangle", Expected: "square,triangle", Matches: true,
	}}
	if err := response.ValidateSemantics(request); err != nil {
		t.Fatalf("visual response with evidence rejected: %v", err)
	}
}

func TestCheckResponseValidateSemantics_RejectsVerdictContradiction(t *testing.T) {
	t.Parallel()
	response := validCheckResponse()
	response.Decision = CheckDecisionIncorrect
	if err := response.ValidateSemantics(CheckRequest{}); err == nil {
		t.Fatal("incorrect verdict with equal normalized and expected answers was accepted")
	}
}

func TestKnownTesterRegressions(t *testing.T) {
	t.Run("1 fraction coloring without counted evidence is rejected", func(t *testing.T) {
		request := CheckRequest{TaskStruct: TaskStructCheck{VisualFacts: []VisualFact{{Kind: "fraction_colored_parts"}}}}
		if err := validCheckResponse().ValidateSemantics(request); err == nil {
			t.Fatal("false-positive visual verdict was accepted")
		}
	})
	t.Run("2 correct fraction coloring with evidence is accepted", func(t *testing.T) {
		request := CheckRequest{TaskStruct: TaskStructCheck{VisualFacts: []VisualFact{{Kind: "fraction_colored_parts"}}}}
		response := validCheckResponse()
		response.Debug.VisualEvidence = []CheckVisualEvidence{{ObjectID: "colored_parts", Observed: "4", Expected: "4", Matches: true}}
		if err := response.ValidateSemantics(request); err != nil {
			t.Fatalf("verified visual verdict rejected: %v", err)
		}
	})
	t.Run("3 wrong boxes answer remains an evaluated incorrect verdict", func(t *testing.T) {
		response := validCheckResponse()
		student, expected := "6", "5"
		response.Decision = CheckDecisionIncorrect
		response.Debug.NormalizedAnswer = &student
		response.Debug.ExpectedAnswer = &expected
		if err := response.ValidateSemantics(CheckRequest{}); err != nil {
			t.Fatalf("valid incorrect verdict rejected: %v", err)
		}
	})
	t.Run("4 rent final answer contradiction is invalidated", func(t *testing.T) {
		assertParseContradiction(t, "27444 + 32646 = 60090 руб.", "59844")
	})
	t.Run("5 venn verdict without per-point evidence is rejected", func(t *testing.T) {
		request := CheckRequest{TaskStruct: TaskStructCheck{VisualFacts: []VisualFact{{Kind: "venn_object"}}}}
		if err := validCheckResponse().ValidateSemantics(request); err == nil {
			t.Fatal("generic Venn verdict was accepted")
		}
	})
	t.Run("6 expression final answer contradiction is invalidated", func(t *testing.T) {
		assertParseContradiction(t, "Последнее действие = 35", "40")
	})
	t.Run("7 partial hint plan coverage is rejected", func(t *testing.T) {
		response := validHintResponse()
		response.Items[0].PlanCoverage.PlanStepsCovered = 2
		if err := response.ValidateAgainstRequest(hintValidationRequest()); err == nil {
			t.Fatal("2/5 plan coverage was accepted")
		}
	})
}

func assertParseContradiction(t *testing.T, step string, finalAnswer interface{}) {
	t.Helper()
	response := ParseResponse{Items: []ParseItem{{SolutionInternal: SolutionInternal{
		SolutionSteps: []string{step}, FinalAnswer: finalAnswer,
	}}}}
	if got := response.ValidateItems(); got != 1 {
		t.Fatalf("ValidateItems()=%d, want 1", got)
	}
}

func hintValidationRequest() HintRequest {
	return HintRequest{
		Task: ParseTask{Subject: SubjectMath},
		Items: []ParseItem{{
			ItemId:           "i1",
			HintPolicy:       HintPolicy{MaxHints: 2},
			SolutionInternal: SolutionInternal{Plan: []string{"step1", "step2", "step3", "step4", "step5"}},
		}},
	}
}

func validHintResponse() HintResponse {
	return HintResponse{Items: []HintItem{{
		ItemId:       "i1",
		PlanCoverage: PlanCoverage{PlanStepsTotal: 5, PlanStepsCovered: 5},
		Hints:        []Hint{{Level: HintL1, HintText: "Идея"}, {Level: HintL2, HintText: "Полный план"}},
	}}}
}

func validCheckResponse() CheckResponse {
	confidence := 0.9
	expected := "4"
	normalized := "4"
	reason := "independent_verification"
	method := "full_recalculation"
	consistent := true
	complete := true
	return CheckResponse{
		Status: CheckStatusEvaluated, CanEvaluate: true, Decision: CheckDecisionCorrect,
		Confidence: &confidence,
		Debug: &CheckDebug{
			NormalizedAnswer: &normalized, ExpectedAnswer: &expected, DecisionReason: &reason, VerificationMethod: &method,
			ParseConsistent: &consistent, VerificationComplete: &complete,
		},
	}
}
