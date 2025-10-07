package prompt

import _ "embed"

// ParseSchema — содержимое parse.schema.json, встраивается в бинарник.
//
//go:embed parse.schema.json
var ParseSchema string
