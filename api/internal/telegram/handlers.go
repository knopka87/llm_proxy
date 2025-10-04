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
		txt, err := eng.Recognize(context.Background(), img, ocr.Options{
			Langs: []string{"ru", "en"},
			// Model: можно настроить по /engine <name> <model>
		})
		if err != nil {
			r.SendError(cid, err)
			return
		}
		if strings.TrimSpace(txt) == "" {
			txt = "(пусто)"
		}
		r.SendResult(cid, txt)
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
