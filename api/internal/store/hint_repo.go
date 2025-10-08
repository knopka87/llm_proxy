package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"child-bot/api/internal/ocr"
)

type HintRepo struct{ DB *sql.DB }

func NewHintRepo(db *sql.DB) *HintRepo { return &HintRepo{DB: db} }

// Find возвращает кэш подсказки указанного уровня (1..3) для (imageHash, engine, model).
// Если maxAge > 0 и запись старше, вернёт sql.ErrNoRows (чтобы вызвать LLM заново).
func (r *HintRepo) Find(ctx context.Context, imageHash, engine, model string, level int, maxAge time.Duration) (ocr.HintResult, error) {
	const q = `select hint_json, created_at
	           from hints_cache
	           where image_hash=$1 and engine=$2 and model=$3 and level=$4`
	var (
		js []byte
		ts time.Time
	)
	if err := r.DB.QueryRowContext(ctx, q, imageHash, engine, model, level).Scan(&js, &ts); err != nil {
		return ocr.HintResult{}, err
	}
	if maxAge > 0 && time.Since(ts) > maxAge {
		return ocr.HintResult{}, sql.ErrNoRows
	}
	var hr ocr.HintResult
	if err := json.Unmarshal(js, &hr); err != nil {
		// Если кэш битый — считаем, что нет валидной записи
		return ocr.HintResult{}, sql.ErrNoRows
	}
	return hr, nil
}

// Upsert сохраняет/обновляет подсказку указанного уровня.
// PK: (image_hash, engine, model, level).
func (r *HintRepo) Upsert(ctx context.Context, imageHash, engine, model string, level int, hr ocr.HintResult) error {
	js, _ := json.Marshal(hr)
	const q = `
insert into hints_cache(image_hash, engine, model, level, hint_json)
values ($1,$2,$3,$4,$5)
on conflict (image_hash, engine, model, level)
do update set hint_json=excluded.hint_json, created_at=now()`
	_, err := r.DB.ExecContext(ctx, q, imageHash, engine, model, level, js)
	return err
}
