package gpt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"llm-proxy/api/internal/util"
	"llm-proxy/api/internal/v1/ocr/types"
)

const DETECT = "detect"

func (e *Engine) Detect(ctx context.Context, in types.DetectInput) (types.DetectResult, error) {
	if e.APIKey == "" {
		return types.DetectResult{}, fmt.Errorf("OPENAI_API_KEY not set")
	}

	model := e.GetModel()
	// TODO переделать на отдельный env
	model = "gpt-4.1-mini"

	// accept raw base64 or data: URL
	imgBytes, mimeFromDataURL, _ := util.DecodeBase64MaybeDataURL(in.ImageB64)
	if len(imgBytes) == 0 {
		raw, err := base64.StdEncoding.DecodeString(in.ImageB64)
		if err != nil {
			return types.DetectResult{}, fmt.Errorf("openai detect: invalid image base64")
		}
		imgBytes = raw
	}
	mime := util.PickMIME(in.Mime, mimeFromDataURL, imgBytes)
	if !isOpenAIImageMIME(mime) {
		return types.DetectResult{}, fmt.Errorf("openai detect: unsupported MIME %s (need image/jpeg|png|webp)", mime)
	}
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(imgBytes)

	system := `DETECT — system prompt v5.2 (text-only, PII OFF per MVP)

Роль: ты — модуль DETECT сервиса «Объяснятель ДЗ». Единственная задача — извлечь задачи с фото/скана и вернуть ОДИН корневой объект строго в JSON по detect.schema.json.

⚙️ Формат ответа
• Ответ только JSON по detect.schema.json.
• Никаких решений, пояснений, рассуждений, Markdown и префиксов.
• Схема передаётся параметром вызова response_format: { type: "json_schema", json_schema: detect.schema.json } — не вставляй схему в текст.
• Конец ответа — закрывающая фигурная скобка корневого объекта. Любой текст вне JSON — ошибка.

📏 Жёсткие правила
1) VERBATIM. Нельзя изменять исходный текст: порядок слов, регистр, «е/ё», пунктуацию, тип/кол-во пробелов (включая NBSP), табы, переносы строк.
2) NUMBERS. Сохраняй разрядные пробелы («68 000», «3 516 997») и их тип. Не склеивать «68000», не заменять пробелы.
3) OPERATORS. Сохраняй исходные символы операций: «·/×», «: / ÷», «+ / −» как в источнике. Запрещены замены на *, x, / и т.п.
4) NUMBERING. Сохраняй оригинальные номера и подпункты (а), б), 1), 2), …). Не перенумеровывать, не добавлять отсутствующие, не исправлять.
5) BLOCKS/ITEMS. Каждый визуальный блок верни в blocks[].block_raw (verbatim). Если блок явно состоит из атомов — разложи их в items_raw[] с group_id = block_id. Конкатенация всех items_raw одного group_id ДОЛЖНА в точности равняться block_raw.
6) LAYOUT. Если виден «столбик», сетка, «□», линейки: добавь оба слоя — layout_raw (фиксированная ширина/моноширинный текст) и semantic_raw (строки/колонки/позиции символов). Если неприменимо — не заполняй эти поля.
7) FLAGS (PII OFF в MVP). Не выполняй распознавание лиц/ФИО/телефонов и т.п.; не используй/не заполняй флаги, связанные с PII/лицами. Любые изображения на полях (рисунки, клипарт, герои учебников, схемы, пиктограммы) НЕ считать лицами и не влияют на ответ.
8) НИЧЕГО ЛИШНЕГО. Никаких нормализаций, исправлений орфографии, домыслов, переводов, подсказок или ответов на задачи.

🧭 Разбиение
• Разделяй задания по явным визуальным признакам (номера, заголовки, литеры, абзацы, колонки). Не дели произвольно.
• Подпункты/литеры фиксируй ровно настолько, насколько они есть в источнике (без добавлений/объединений).
• Если разбиение на items_raw неочевидно — оставь весь текст в block_raw без атомизации; не выдумывай структуру.

✅ Самопроверки перед отдачей JSON
• Для каждого block_id: join(items_raw[group_id]) == block_raw (побайтно).
• Сохранены исходные операторы и разрядные пробелы → operators_strict=true, thousands_space_preserved=true (если применимо).
• NUMBERING соответствует источнику (оригинальные номера/литеры без сдвигов).
• layout_raw/semantic_raw присутствуют только когда действительно необходимы.

Верни строго JSON по схеме detect. Любой текст вне JSON — ошибка. Никаких комментариев, заголовков и пояснений.
`

	// system = "Верни строго JSON по схеме detect. Любой текст вне JSON — ошибка. Никаких комментариев, заголовков и пояснений."
	schema, err := util.LoadPromptSchema(DETECT, e.Version())
	if err != nil {
		return types.DetectResult{}, err
	}
	util.FixJSONSchemaStrict(schema)

	user := "Ответ строго JSON по схеме. Без комментариев."
	if in.GradeHint >= 1 && in.GradeHint <= 4 {
		user += fmt.Sprintf(" grade_hint=%d", in.GradeHint)
	}

	body := map[string]any{
		"model": model,
		"input": []any{
			map[string]any{
				"role": "system",
				"content": []any{
					map[string]any{"type": "input_text", "text": system},
				},
			},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": user},
					map[string]any{"type": "input_image", "image_url": dataURL},
				},
			},
		},
		"temperature": 0,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   DETECT,
				"strict": true,
				"schema": schema,
			},
		},
	}

	if strings.Contains(model, "gpt-5") {
		body["temperature"] = 1
	}

	payload, _ := json.Marshal(body)
	log.Print(string(payload))
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/responses", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	start := time.Now()
	resp, err := e.httpc.Do(req)
	t := time.Since(start).Milliseconds()
	log.Printf("detect time: %d", t)
	if err != nil {
		return types.DetectResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		x, _ := io.ReadAll(resp.Body)
		return types.DetectResult{}, fmt.Errorf("openai detect %d: %s", resp.StatusCode, strings.TrimSpace(string(x)))
	}

	raw, _ := io.ReadAll(resp.Body)
	out, err := util.ExtractResponsesText(bytes.NewReader(raw))
	if err != nil || strings.TrimSpace(out) == "" {
		// fallback to manual extraction from Responses API envelope
		out = fallbackExtractResponsesText(raw)
	}
	out = util.StripCodeFences(strings.TrimSpace(out))
	if out == "" {
		return types.DetectResult{}, fmt.Errorf("responses: empty output; body=%s", truncateBytes(raw, 1024))
	}
	var r types.DetectResult
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		return types.DetectResult{}, fmt.Errorf("openai detect: bad JSON: %w", err)
	}
	return r, nil
}
