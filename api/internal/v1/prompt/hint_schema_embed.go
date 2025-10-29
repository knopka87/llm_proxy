package prompt

import _ "embed"

// HintSchema — содержимое hint.schema.json, встраивается в бинарник.
//
//go:embed hint.schema.json
var HintSchema string
