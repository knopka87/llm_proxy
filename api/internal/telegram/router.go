package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"child-bot/api/internal/ocr"
	"child-bot/api/internal/store"
)

type Router struct {
	Bot        *tgbotapi.BotAPI
	EngManager *ocr.Manager
	ParseRepo  *store.ParseRepo
	HintRepo   *store.HintRepo

	// Defaults / display models
	GeminiModel   string
	OpenAIModel   string
	DeepseekModel string
}

func (r *Router) HandleCommand(upd tgbotapi.Update) {
	cid := upd.Message.Chat.ID
	switch upd.Message.Command() {
	case "start":
		r.send(cid, "Пришли фото задачи — верну распознанный текст.\nКоманды: /health, /engine")
	case "health":
		r.send(cid, "✅ OK")
	case "engine":
		args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(upd.Message.Text, "/engine")))
		if len(args) == 0 {
			cur := r.EngManager.Get(cid).Name()
			r.send(cid, "Текущий движок: "+cur+
				"\nИспользование:\n/engine yandex\n/engine gemini [model]\n/engine gpt [model]\n/engine deepseek [model]")
			return
		}
		name := strings.ToLower(args[0])
		var mdl string
		if len(args) > 1 {
			mdl = args[1]
		}
		switch name {
		case "yandex", "gemini", "gpt", "deepseek":
			r.send(cid, "Ок, переключаю на: "+name+func() string {
				if mdl != "" {
					return " (" + mdl + ")"
				}
				return ""
			}())
			// фактическое переключение делается в handlers.go (по /engine ...), там есть привязка к конкретным инстансам
		default:
			r.send(cid, "Неизвестный движок. Доступны: yandex | gemini | gpt | deepseek")
		}
	default:
		r.send(cid, "Неизвестная команда")
	}
}

func (r *Router) HandleUpdate(upd tgbotapi.Update, engines Engines) {
	// callback-кнопки
	if upd.CallbackQuery != nil {
		r.handleCallback(*upd.CallbackQuery, engines)
		return
	}
	if upd.Message == nil {
		return
	}
	cid := upd.Message.Chat.ID

	// если ждём текстовую правку после "Нет"
	if r.hasPendingCorrection(cid) && upd.Message.Text != "" {
		r.applyTextCorrectionThenShowHints(cid, upd.Message.Text, engines)
		return
	}

	// выбор пункта при multiple tasks (1..N)
	if v, ok := pendingChoice.Load(cid); ok && upd.Message.Text != "" {
		briefs := v.([]string)
		if n, err := strconv.Atoi(strings.TrimSpace(upd.Message.Text)); err == nil && n >= 1 && n <= len(briefs) {
			if ctxv, ok2 := pendingCtx.Load(cid); ok2 {
				pendingChoice.Delete(cid)
				pendingCtx.Delete(cid)
				sc := ctxv.(*selectionContext)
				r.send(cid, fmt.Sprintf("Ок, беру задание: %s — обрабатываю.", briefs[n-1]))
				r.runParseAndMaybeConfirm(context.Background(), cid, sc, engines, n-1, briefs[n-1])
				return
			}
			pendingChoice.Delete(cid)
			r.send(cid, "Не нашёл предыдущее изображение. Пришлите фото ещё раз.")
			return
		}
	}

	// переключение движка (если было)
	if upd.Message.IsCommand() && strings.HasPrefix(upd.Message.Text, "/engine") {
		r.handleEngineCommand(cid, upd.Message.Text, engines) // реализуйте как у вас
		return
	}
	if upd.Message.IsCommand() {
		r.HandleCommand(upd) // ваше
		return
	}

	// фото
	if len(upd.Message.Photo) > 0 {
		r.acceptPhoto(*upd.Message, engines)
	}
}

func (r *Router) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	_, _ = r.Bot.Send(msg)
}

func (r *Router) SendResult(chatID int64, text string) {
	if len(text) > 3900 {
		text = text[:3900] + "…"
	}
	r.send(chatID, "📝 Распознанный текст:\n\n"+text)
}

func (r *Router) SendError(chatID int64, err error) {
	r.send(chatID, fmt.Sprintf("Ошибка OCR: %v", err))
}

// handleEngineCommand парсит команду /engine и переключает движок для чата.
// Форматы:
//
//	/engine yandex
//	/engine gemini [model]
//	/engine gpt [model]
//	/engine deepseek
func (r *Router) handleEngineCommand(chatID int64, cmd string, engines Engines) {
	args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(cmd, "/engine")))
	if len(args) == 0 {
		r.send(chatID, "Использование: /engine {yandex|gemini|gpt|deepseek} [model]")
		return
	}
	name := strings.ToLower(args[0])
	var modelArg string
	if len(args) > 1 {
		modelArg = strings.TrimSpace(args[1])
	}

	// Вспомогательный интерфейс: некоторые движки могут уметь переключать дефолтную модель.
	type modelSetter interface{ SetModel(string) }

	switch name {
	case "yandex":
		// OCR-только; для подсказок всё равно будет выбран LLM (gemini/gpt) при нажатии кнопки
		r.EngManager.Set(chatID, engines.Yandex)
		r.send(chatID, "✅ Движок: yandex (OCR). Подсказки будут через выбранный LLM при необходимости.")

	case "gemini":
		eng := engines.Gemini
		if eng == nil {
			r.send(chatID, "❌ Gemini не настроен.")
			return
		}
		// Опционально переключим модель, если движок это поддерживает
		if modelArg != "" {
			if ms, ok := any(eng).(modelSetter); ok {
				ms.SetModel(modelArg)
			}
		}
		r.EngManager.Set(chatID, eng)

		r.send(chatID, "✅ Движок: gemini ("+eng.GetModel()+").")

	case "gpt", "openai":
		eng := engines.OpenAI
		if eng == nil {
			r.send(chatID, "❌ OpenAI GPT не настроен.")
			return
		}
		if modelArg != "" {
			if ms, ok := any(eng).(modelSetter); ok {
				ms.SetModel(modelArg)
			}
		}
		r.EngManager.Set(chatID, eng)

		r.send(chatID, "✅ Движок: gpt ("+eng.GetModel()+").")

	case "deepseek":
		r.EngManager.Set(chatID, engines.Deepseek)
		r.send(chatID, "⚠️ DeepSeek не анализирует изображения. Для подсказок используйте /engine gemini или /engine gpt.")

	default:
		r.send(chatID, "Неизвестный движок. Доступны: yandex | gemini | gpt | deepseek")
	}
}

// Показ запроса подтверждения распознанного текста
func (r *Router) askParseConfirmation(chatID int64, pr ocr.ParseResult) {
	var b strings.Builder
	b.WriteString("Я так прочитал задание. Всё верно?\n")
	if s := strings.TrimSpace(pr.RawText); s != "" {
		b.WriteString("```\n")
		b.WriteString(s)
		b.WriteString("\n```\n")
	}
	if q := strings.TrimSpace(pr.Question); q != "" {
		b.WriteString("\nВопрос: ")
		b.WriteString(esc(q))
		b.WriteString("\n")
	}

	msg := tgbotapi.NewMessage(chatID, b.String())
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = makeParseConfirmKeyboard()
	_, _ = r.Bot.Send(msg)
}

// PhotoAcceptedText — первый ответ после получения фото/первой страницы альбома.
func (r *Router) PhotoAcceptedText() string {
	return "Фото принято. Если задание на нескольких фото — просто пришлите их подряд, я склею страницы перед обработкой."
}
