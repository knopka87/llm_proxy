package prompt

import _ "embed"

// NormalizeSchema — содержимое normalize.schema.json, встраивается в бинарник.
//
//go:embed normalize.schema.json
var NormalizeSchema string
