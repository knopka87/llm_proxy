package prompt

import _ "embed"

// OcrSchema — содержимое ocr.schema.json, встраивается в бинарник.
//
//go:embed ocr.schema.json
var OcrSchema string
