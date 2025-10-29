package prompt

import _ "embed"

// CheckSolutionSchema — содержимое check.schema.json, встраивается в бинарник.
//
//go:embed check.schema.json
var CheckSolutionSchema string
