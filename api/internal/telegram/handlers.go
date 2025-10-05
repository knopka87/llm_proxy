package telegram

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"child-bot/api/internal/ocr"
	"child-bot/api/internal/ocr/deepseek"
	"child-bot/api/internal/ocr/gemini"
	"child-bot/api/internal/ocr/openai"
	"child-bot/api/internal/ocr/yandex"
)

type Engines struct {
	Yandex   *yandex.Engine
	Gemini   *gemini.Engine
	OpenAI   *openai.Engine
	Deepseek *deepseek.Engine
}

func (r *Router) HandleUpdate(upd tgbotapi.Update, engines Engines) {
	if upd.Message == nil {
		return
	}
	cid := upd.Message.Chat.ID

	if upd.Message.IsCommand() {
		if strings.HasPrefix(upd.Message.Text, "/engine") {
			// Переключение движка
			args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(upd.Message.Text, "/engine")))
			if len(args) == 0 {
				r.HandleCommand(upd)
				return
			}
			name := strings.ToLower(args[0])
			var mdl string
			if len(args) > 1 {
				mdl = args[1]
			}
			switch name {
			case "yandex":
				r.EngManager.Set(cid, engines.Yandex)
			case "gemini":
				if mdl != "" {
					engines.Gemini.Model = mdl
				}
				r.EngManager.Set(cid, engines.Gemini)
			case "gpt":
				if mdl != "" {
					engines.OpenAI.Model = mdl
				}
				r.EngManager.Set(cid, engines.OpenAI)
			case "deepseek":
				if mdl != "" {
					engines.Deepseek.Model = mdl
				}
				r.EngManager.Set(cid, engines.Deepseek)
				r.send(cid, "⚠️ Внимание: DeepSeek Chat API не умеет работать с картинками. Для OCR используйте /engine yandex | gemini | gpt.")
			default:
				r.send(cid, "Неизвестный движок")
				return
			}
			r.send(cid, fmt.Sprintf("✅ Движок: %s", name))
			return
		}
		// другие команды
		r.HandleCommand(upd)
		return
	}

	// Фото
	if len(upd.Message.Photo) > 0 {
		r.send(cid, r.PhotoAcceptedText())
		// берём самое большое превью
		ph := upd.Message.Photo[len(upd.Message.Photo)-1]
		file, err := r.Bot.GetFile(tgbotapi.FileConfig{FileID: ph.FileID})
		if err != nil {
			r.SendError(cid, err)
			return
		}
		url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", r.Bot.Token, file.FilePath)
		img, err := download(url)
		if err != nil {
			r.SendError(cid, err)
			return
		}

		eng := r.EngManager.Get(cid)
		res, err := eng.Analyze(context.Background(), img, ocr.Options{
			Langs: []string{"ru", "en"},
			// Model: можно настроить по /engine <name> <model>
		})
		if err != nil {
			r.SendError(cid, err)
			return
		}
		switch eng.Name() {
		case "yandex":
			// Только транскрипт
			txt := strings.TrimSpace(res.Text)
			if txt == "" {
				txt = "(пусто)"
			}
			r.SendResult(cid, txt)
		default:
			// Аналитический ответ
			var b strings.Builder
			if strings.TrimSpace(res.Text) != "" {
				b.WriteString("📄 *Текст задачи:*\n")
				b.WriteString("```\n")
				b.WriteString(res.Text)
				b.WriteString("\n```\n\n")
			}
			if res.FoundSolution {
				switch res.SolutionVerdict {
				case "correct":
					b.WriteString("✅ Задача решена верно.\n\n")
				case "incorrect":
					b.WriteString("⚠️ В решении есть ошибка.\n")
					if strings.TrimSpace(res.SolutionNote) != "" {
						b.WriteString("Подсказка где/какого рода: ")
						b.WriteString(res.SolutionNote)
						b.WriteString("\n\n")
					} else {
						b.WriteString("\n")
					}
				default:
					b.WriteString("ℹ️ Решение обнаружено, но проверка неуверенна.\n\n")
				}
			} else {
				b.WriteString("ℹ️ На изображении нет готового решения.\n\n")
			}
			if len(res.Hints) > 0 {
				b.WriteString("💡 *Подсказки (L1→L3):*\n")
				for i, h := range res.Hints {
					fmt.Fprintf(&b, "%d) %s\n", i+1, h)
				}
			}

			msg := tgbotapi.NewMessage(cid, b.String())
			msg.ParseMode = "Markdown"
			_, _ = r.Bot.Send(msg)
		}
	}
}

func download(url string) ([]byte, error) {
	resp, err := httpClient().Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 60 * 1e9}
}
