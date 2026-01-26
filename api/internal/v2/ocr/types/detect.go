package types

// DetectRequest — DETECT.request.v1
// Required: image. Optional: locale ("ru-RU" | "en-US").
type DetectRequest struct {
	Image  string `json:"image"`            // Image handle (URL or base64 id)
	Locale string `json:"locale,omitempty"` // "ru-RU" | "en-US"
}

// Subject — enum for subject classification (shared by Detect and Parse)
type Subject string

const (
	SubjectMath       Subject = "math"
	SubjectRu         Subject = "ru"
	SubjectEn         Subject = "en"
	SubjectWorld      Subject = "world"
	SubjectLiterature Subject = "literature"
	SubjectOther      Subject = "other"
)

// QualityIssue — enum for quality issues
type QualityIssue string

const (
	IssueBlur         QualityIssue = "blur"
	IssueGlare        QualityIssue = "glare"
	IssueLowLight     QualityIssue = "low_light"
	IssueCutOff       QualityIssue = "cut_off"
	IssueOccludedText QualityIssue = "occluded_text"
	IssueTooSmallText QualityIssue = "too_small_text"
	IssueSkewed       QualityIssue = "skewed"
	IssueNoTextFound  QualityIssue = "no_text_found"
	IssueMultiPages   QualityIssue = "multiple_pages"
	IssueOther        QualityIssue = "other"
)

// Quality — image quality assessment
type Quality struct {
	RecommendRetake bool           `json:"recommend_retake"`
	Issues          []QualityIssue `json:"issues"`
}

// Classification — subject classification result
type Classification struct {
	SubjectCandidate Subject `json:"subject_candidate"` // "math" | "ru" | "en" | "world" | "literature" | "other"
	Confidence       float64 `json:"confidence"`
}

// DetectResponse — DETECT_OUTPUT
// Required: schema_version, quality, classification.
type DetectResponse struct {
	SchemaVersion  string         `json:"schema_version"`
	Quality        Quality        `json:"quality"`
	Classification Classification `json:"classification"`
}
