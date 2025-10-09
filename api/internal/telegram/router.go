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
		r.send(cid, "Пришли фото задачи — верну распознанный текст и подскажу, с чего начать.\nКоманды: /health, /engine")
	case "health":
		r.send(cid, "✅ OK")
	case "engine":
		// До сюда обычно не дойдём — /engine обрабатывается раньше в HandleUpdate.
		args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(upd.Message.Text, "/engine")))
		if len(args) == 0 {
			cur := r.EngManager.Get(cid).Name()
			r.send(cid, "Текущий движок: "+cur+
				"\nИспользование:\n/engine yandex\n/engine gemini [model]\n/engine gpt [model]\n/engine deepseek")
			return
		}
		r.send(cid, "Ок, переключаю…")
	default:
		r.send(cid, "Неизвестная команда")
	}
}

func (r *Router) HandleUpdate(upd tgbotapi.Update, engines Engines) {
	// 1) Callback-кнопки
	if upd.CallbackQuery != nil {
		r.handleCallback(*upd.CallbackQuery, engines)
		return
	}

	// 2) Сообщений нет — выходим
	if upd.Message == nil {
		return
	}
	cid := upd.Message.Chat.ID

	// 3) Если ждём текстовую правку после «Нет» — приоритетно принимаем её
	if r.hasPendingCorrection(cid) && upd.Message.Text != "" {
		r.applyTextCorrectionThenShowHints(cid, upd.Message.Text, engines)
		return
	}

	// 4) «Жёсткий» режим: если ждём фото (решение/новая задача) и пришёл произвольный ТЕКСТ — мягко игнорируем.
	// Команды разрешаем, чтобы можно было переключать движки/проверять health.
	if upd.Message.Text != "" && !upd.Message.IsCommand() {
		switch getMode(cid) {
		case "await_solution":
			r.send(cid, "Я жду фото с вашим решением. Пожалуйста, пришлите фото.")
			return
		case "await_new_task":
			r.send(cid, "Я жду фото новой задачи. Пожалуйста, пришлите фото.")
			return
		}
	}

	// 5) Ветвь выбора пункта при multiple tasks (ожидаем число 1..N)
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
		// иначе ждём корректный номер
	}

	// 6) Команды (в т.ч. /engine)
	if upd.Message.IsCommand() && strings.HasPrefix(upd.Message.Text, "/engine") {
		r.handleEngineCommand(cid, upd.Message.Text, engines)
		return
	}
	if upd.Message.IsCommand() {
		r.HandleCommand(upd)
		return
	}

	// 7) Фото/альбом — это снимает «режим ожидания фото»
	if len(upd.Message.Photo) > 0 {
		clearMode(cid) // получили фото — разблокируем пайплайн
		r.acceptPhoto(*upd.Message, engines)
		return
	}

	// 8) Остальное — игнорируем
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

	// Некоторым движкам можно менять модель «на лету»
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
