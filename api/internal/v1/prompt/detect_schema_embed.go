package prompt

import _ "embed"

// DetectSchema — содержимое detect.schema.json, встраивается в бинарник.
//
//go:embed detect.schema.json
var DetectSchema string
