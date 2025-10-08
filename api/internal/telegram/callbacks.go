package telegram

import (
	"context"
	"fmt"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"child-bot/api/internal/ocr"
	"child-bot/api/internal/util"
)

func (r *Router) handleCallback(cb tgbotapi.CallbackQuery, engines Engines) {
	cid := cb.Message.Chat.ID
	data := cb.Data
	_, _ = r.Bot.Request(tgbotapi.NewCallback(cb.ID, "")) // ack

	switch data {
	case "hint_next":
		r.onHintNext(cid, cb.Message.MessageID, engines)
	case "parse_yes":
		r.onParseYes(cid, engines, cb.Message.MessageID)
	case "parse_no":
		r.onParseNo(cid, cb.Message.MessageID)
	}
}

func (r *Router) onParseYes(chatID int64, engines Engines, msgID int) {
	v, ok := parseWait.Load(chatID)
	if !ok {
		r.send(chatID, "Контекст подтверждения не найден.")
		return
	}
	parseWait.Delete(chatID)
	p := v.(*parsePending)

	llm := r.resolveEngineByName(p.LLM, engines)
	if llm == nil {
		r.send(chatID, "LLM-движок недоступен.")
		return
	}
	imgHash := util.SHA256Hex(p.Sc.Image)

	_ = r.ParseRepo.MarkAccepted(context.Background(), imgHash, llm.Name(), llm.GetModel(), "user_yes")
	// убрать клавиатуру
	edit := tgbotapi.NewEditMessageReplyMarkup(chatID, msgID, tgbotapi.InlineKeyboardMarkup{})
	_, _ = r.Bot.Send(edit)
	// продолжить
	r.showTaskAndPrepareHints(chatID, p.Sc, p.PR, llm)
}

func (r *Router) onParseNo(chatID int64, msgID int) {
	edit := tgbotapi.NewEditMessageReplyMarkup(chatID, msgID, tgbotapi.InlineKeyboardMarkup{})
	_, _ = r.Bot.Send(edit)
	r.send(chatID, "Напишите, пожалуйста, текст задания так, как он должен быть прочитан (без ответа). Это поможет дать корректные подсказки.")
	// остаёмся в состоянии parseWait — следующий текст примем как корректировку
}

func (r *Router) onHintNext(chatID int64, msgID int, engines Engines) {
	v, ok := hintState.Load(chatID)
	if !ok {
		r.send(chatID, "Подсказки недоступны: сначала пришлите фото задания.")
		return
	}
	hs := v.(*hintSession)
	if hs.NextLevel > 3 {
		edit := tgbotapi.NewEditMessageReplyMarkup(chatID, msgID, tgbotapi.InlineKeyboardMarkup{})
		_, _ = r.Bot.Send(edit)
		r.send(chatID, "Все подсказки уже показаны.")
		return
	}
	llm := r.resolveEngineByName(hs.EngineName, engines)
	if llm == nil {
		r.send(chatID, "LLM-движок недоступен.")
		return
	}

	imgHash := util.SHA256Hex(hs.Image)
	level := hs.NextLevel

	// кэш подсказок
	if hr, err := r.HintRepo.Find(context.Background(), imgHash, hs.EngineName, hs.Model, level, 90*24*time.Hour); err == nil {
		r.send(chatID, formatHint(level, hr))
	} else {
		in := ocr.HintInput{
			Level:             lvlToConst(level),
			RawText:           hs.Parse.RawText,
			Subject:           hs.Parse.Subject,
			TaskType:          hs.Parse.TaskType,
			Grade:             hs.Parse.Grade,
			SolutionShape:     hs.Parse.SolutionShape,
			TerminologyLevel:  levelTerminology(level),
			SubjectConfidence: hs.Detect.SubjectConfidence,
		}
		hrNew, err := llm.Hint(context.Background(), in)
		if err != nil {
			r.send(chatID, fmt.Sprintf("Не удалось получить подсказку L%d: %v", level, err))
			return
		}
		_ = r.HintRepo.Upsert(context.Background(), imgHash, hs.EngineName, hs.Model, level, hrNew)
		r.send(chatID, formatHint(level, hrNew))
	}

	hs.NextLevel++
	if hs.NextLevel > 3 {
		edit := tgbotapi.NewEditMessageReplyMarkup(chatID, msgID, tgbotapi.InlineKeyboardMarkup{})
		_, _ = r.Bot.Send(edit)
	}
}

func lvlToConst(n int) ocr.HintLevel {
	switch n {
	case 1:
		return ocr.HintL1
	case 2:
		return ocr.HintL2
	default:
		return ocr.HintL3
	}
}

func levelTerminology(n int) string {
	switch n {
	case 1:
		return "none"
	case 2:
		return "light"
	default:
		return "teacher"
	}
}
