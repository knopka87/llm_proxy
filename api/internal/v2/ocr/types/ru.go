package types

// --- PARSE_RU types ---

// ParseRURequest — входной запрос PARSE_RU
type ParseRURequest struct {
	Image            string `json:"image"`
	TaskId           string `json:"task_id"`
	Grade            int    `json:"grade"`
	SubjectCandidate string `json:"subject_candidate"`
	Locale           string `json:"locale"`
}

// RUParseMeta — метаданные распознавания
type RUParseMeta struct {
	Readable        bool   `json:"readable"`
	RecommendRetake bool   `json:"recommend_retake"`
	TaskTextClean   string `json:"task_text_clean"`
	HasChildAnswer  bool   `json:"has_child_answer"`
	HasHandwriting  bool   `json:"has_handwriting"`
}

// RUActionPlan — план действий из PARSE_RU
type RUActionPlan struct {
	Schema  string     `json:"schema"`
	Subject string     `json:"subject"`
	Grade   int        `json:"grade"`
	TaskKind string    `json:"task_kind"`
	Coverage string    `json:"coverage"`
	Actions  []RUAction `json:"actions"`
}

// RUAction — одно действие в задании
type RUAction struct {
	ActionID            string   `json:"action_id"`
	TaskAction          string   `json:"task_action"`
	CheckMode           string   `json:"check_mode"`
	RuleFamily          string   `json:"rule_family"`
	Reliability         string   `json:"reliability"`
	TemplateIDCandidate *string  `json:"template_id_candidate"`
	ClassificationBasis *string  `json:"classification_basis"`
	Items               []string `json:"items"`
	TargetFormat        string   `json:"target_format"`
	ExpectedAnswerType  string   `json:"expected_answer_type"`
	VisualRequirement   string   `json:"visual_requirement"`
	SourceTextRole      string   `json:"source_text_role"`
	SubRuleKeyHint      *string  `json:"sub_rule_key_hint"`
	VisualTargets       []string `json:"visual_targets"`
}

// ParseRUResponse — ответ PARSE_RU
type ParseRUResponse struct {
	ParseMeta  RUParseMeta  `json:"parse_meta"`
	ActionPlan RUActionPlan `json:"action_plan"`
}

// --- HINT_RU types ---

// HintRUCompactInput — компактный payload для HINT_RU
type HintRUCompactInput struct {
	Grade          int                `json:"grade"`
	FlowStatus     string             `json:"flow_status"`
	ActionsPayload []RUHintPayload    `json:"actions_payload"`
	SourceItems    []string           `json:"source_items"`
	AntiGDZBans    []string           `json:"anti_gdz_bans"`
	Limits         RULimits           `json:"limits"`
}

// RUHintPayload — payload для одного действия в HINT
type RUHintPayload struct {
	ActionID        string              `json:"action_id"`
	TemplateID      string              `json:"template_id"`
	TaskAction      string              `json:"task_action"`
	RuleFamily      string              `json:"rule_family"`
	SubRuleKey      *string             `json:"sub_rule_key"`
	Reliability     string              `json:"reliability"`
	HelpStrategy    string              `json:"help_strategy"`
	RelevantRules   []RUCompactRuleHint `json:"relevant_rules"`
	AntiGDZBans     []string            `json:"anti_gdz_bans"`
	SourceItems     []string            `json:"source_items"`
}

// RUCompactRuleHint — компактная карточка правила для HINT
type RUCompactRuleHint struct {
	RuleID        string   `json:"rule_id"`
	Title         string   `json:"title"`
	ShortRule     string   `json:"short_rule"`
	WhenApplies   string   `json:"when_applies"`
	ExamplesOther []string `json:"examples_other"`
}

// RULimits — лимиты для ответа
type RULimits struct {
	MaxHintCards   int `json:"max_hint_cards"`
	MaxRuleButtons int `json:"max_rule_buttons"`
	MaxErrorGroups int `json:"max_error_groups"`
}

// HintRUResponse — ответ HINT_RU
type HintRUResponse struct {
	Status              string             `json:"status"`
	Confidence          string             `json:"confidence"`
	ChildMessage        string             `json:"child_message"`
	RoadmapSteps        []string           `json:"roadmap_steps"`
	HintCards           []RUHintCard       `json:"hint_cards"`
	RuleButtons         []RURuleButton     `json:"rule_buttons"`
	AntiGDZViolationRisk bool             `json:"anti_gdz_violation_risk"`
}

// RUHintCard — карточка подсказки
type RUHintCard struct {
	ActionID               string   `json:"action_id"`
	Title                  string   `json:"title"`
	Explanation            string   `json:"explanation"`
	AlgorithmSteps         []string `json:"algorithm_steps"`
	ExampleOnOtherMaterial *string  `json:"example_on_other_material"`
	ChildQuestion          string   `json:"child_question"`
}

// RURuleButton — кнопка правила
type RURuleButton struct {
	RuleID   string `json:"rule_id"`
	Title    string `json:"title"`
	ActionID string `json:"action_id"`
}

// --- CHECK_RU types ---

// CheckRUCompactInput — компактный payload для CHECK_RU
type CheckRUCompactInput struct {
	Grade          int                 `json:"grade"`
	CheckFlow      string              `json:"check_flow"`
	ActionsPayload []RUCheckPayload    `json:"actions_payload"`
	SourceItems    []string            `json:"source_items"`
	AntiGDZBans    []string            `json:"anti_gdz_bans"`
	Limits         RULimits            `json:"limits"`
}

// RUCheckPayload — payload для одного действия в CHECK
type RUCheckPayload struct {
	ActionID          string                `json:"action_id"`
	TemplateID        string                `json:"template_id"`
	TaskAction        string                `json:"task_action"`
	CheckMode         string                `json:"check_mode"`
	RuleFamily        string                `json:"rule_family"`
	SubRuleKey        *string               `json:"sub_rule_key"`
	Reliability       string                `json:"reliability"`
	AnswerReliability string                `json:"answer_reliability"`
	CheckStrategy     string                `json:"check_strategy"`
	RelevantRules     []RUCompactRuleCheck  `json:"relevant_rules"`
	AntiGDZBans       []string              `json:"anti_gdz_bans"`
	SourceItems       []string              `json:"source_items"`
	CheckItems        []RUCheckItem         `json:"check_items"`
}

// RUCompactRuleCheck — компактная карточка правила для CHECK
type RUCompactRuleCheck struct {
	RuleID      string `json:"rule_id"`
	Title       string `json:"title"`
	ShortRule   string `json:"short_rule"`
	WhenApplies string `json:"when_applies"`
}

// RUCheckItem — проверяемый элемент
type RUCheckItem struct {
	SourceItem        string `json:"source_item"`
	StudentAnswer     string `json:"student_answer"`
	AnswerReadability string `json:"answer_readability"`
}

// CheckRUResponse — ответ CHECK_RU
type CheckRUResponse struct {
	Status              string              `json:"status"`
	Confidence          string              `json:"confidence"`
	ChildMessage        string              `json:"child_message"`
	CheckedActions      []RUCheckedAction   `json:"checked_actions"`
	ErrorGroups         []RUErrorGroup      `json:"error_groups"`
	RuleButtons         []RURuleButton      `json:"rule_buttons"`
	AntiGDZViolationRisk bool              `json:"anti_gdz_violation_risk"`
}

// RUCheckedAction — результат проверки действия
type RUCheckedAction struct {
	ActionID     string `json:"action_id"`
	Result       string `json:"result"`
	ShortComment string `json:"short_comment"`
}

// RUErrorGroup — группа ошибок
type RUErrorGroup struct {
	ActionID          string   `json:"action_id"`
	RuleFamily        string   `json:"rule_family"`
	LocationHint      string   `json:"location_hint"`
	Feedback          string   `json:"feedback"`
	SelfCheckQuestion string   `json:"self_check_question"`
	RuleIDs           []string `json:"rule_ids"`
}
