package telegram

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"child-bot/api/internal/ocr"
)

type Router struct {
	Bot        *tgbotapi.BotAPI
	EngManager *ocr.Manager

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

func (r *Router) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	_, _ = r.Bot.Send(msg)
}

func (r *Router) PhotoAcceptedText() string {
	return "Принял фото, обрабатываю…"
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
