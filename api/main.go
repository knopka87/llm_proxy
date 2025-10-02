package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	tgToken    = mustEnv("TELEGRAM_BOT_TOKEN")
	ycOAuth    = mustEnv("YC_OAUTH_TOKEN")
	folderID   = mustEnv("YC_FOLDER_ID")
	webhookURL = mustEnv("WEBHOOK_URL") // напр.: https://<app>.koyeb.app
	apiKey     = mustEnv("SECRET_KEY")

	httpc    = &http.Client{Timeout: 60 * time.Second}
	iamToken string
	iamExp   time.Time
)

// ----- Request -----
type ocrRecognizeRequest struct {
	Content       string   `json:"content"`                 // base64
	MimeType      string   `json:"mimeType,omitempty"`      // image/jpeg | image/png | application/pdf
	LanguageCodes []string `json:"languageCodes,omitempty"` // ["ru","en"]
	Model         string   `json:"model,omitempty"`         // напр. "page", "markdown", "math-markdown" (если доступно)
}

// ----- Response (минимально необходимая часть) -----
type ocrRecognizeResponse struct {
	TextAnnotation *ocrTextAnnotation `json:"textAnnotation,omitempty"`
	Page           string             `json:"page,omitempty"`
}

type ocrTextAnnotation struct {
	FullText string     `json:"fullText,omitempty"`
	Blocks   []ocrBlock `json:"blocks,omitempty"`
	// прочие поля опущены за ненадобностью
}

type ocrBlock struct {
	Lines []ocrLine `json:"lines,omitempty"`
}

type ocrLine struct {
	Text string `json:"text,omitempty"`
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing env %s", k)
	}
	return v
}

func main() {
	bot, err := tgbotapi.NewBotAPI(tgToken)
	if err != nil {
		log.Fatal(err)
	}
	bot.Debug = false

	path := "/webhook/" + shortHash(tgToken)
	public := strings.TrimRight(webhookURL, "/") + path

	cfg, err := tgbotapi.NewWebhook(public)
	if err != nil {
		log.Fatal(err)
	}
	cfg.DropPendingUpdates = true
	if _, err := bot.Request(cfg); err != nil {
		log.Fatal(err)
	}

	updates := bot.ListenForWebhook(path)

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("telegram webhook bot"))
	})

	go func() {
		for upd := range updates {
			handleUpdate(bot, upd)
		}
	}()

	addr := "0.0.0.0:8080"
	log.Printf("listening on %s; webhook=%s", addr, public)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handleUpdate(bot *tgbotapi.BotAPI, upd tgbotapi.Update) {
	if upd.Message == nil {
		return
	}
	cid := upd.Message.Chat.ID

	if upd.Message.IsCommand() {
		switch upd.Message.Command() {
		case "start":
			send(bot, cid, "Пришли фото задачи — верну распознанный текст. Команды: /health")
		case "health":
			if err := ensureIAM(context.Background()); err != nil {
				send(bot, cid, "⚠️ IAM недоступен: "+err.Error())
			} else {
				send(bot, cid, "✅ OK: Webhook + Yandex Vision")
			}
		default:
			send(bot, cid, "Неизвестная команда")
		}
		return
	}

	// Фото → OCR
	if len(upd.Message.Photo) > 0 {
		send(bot, cid, "Принял фото, обрабатываю…")
		ph := upd.Message.Photo[len(upd.Message.Photo)-1]
		tf, err := bot.GetFile(tgbotapi.FileConfig{FileID: ph.FileID})
		if err != nil {
			send(bot, cid, "Не смог получить файл: "+err.Error())
			return
		}
		url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", tgToken, tf.FilePath)
		img, err := download(url)
		if err != nil {
			send(bot, cid, "Не смог скачать фото: "+err.Error())
			return
		}
		txt, err := yandexOCR(context.Background(), img, []string{"ru", "en"})
		if err != nil {
			send(bot, cid, "Ошибка OCR: "+err.Error())
			return
		}
		if strings.TrimSpace(txt) == "" {
			txt = "(пусто)"
		}
		if len(txt) > 3900 {
			txt = txt[:3900] + "…"
		}
		send(bot, cid, "📝 Распознанный текст:\n\n"+txt)
	}
}

func send(bot *tgbotapi.BotAPI, chatID int64, text string) {
	_, _ = bot.Send(tgbotapi.NewMessage(chatID, text))
}

func download(url string) ([]byte, error) {
	resp, err := httpc.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return io.ReadAll(resp.Body)
}

// ---- OAuth -> IAM (кеш ~11 ч) ----
func ensureIAM(ctx context.Context) error {
	if iamToken != "" && time.Now().Before(iamExp.Add(-1*time.Minute)) {
		return nil
	}
	body := map[string]string{"yandexPassportOauthToken": ycOAuth}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://iam.api.cloud.yandex.net/iam/v1/tokens", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		x, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("iam %d: %s", resp.StatusCode, string(x))
	}
	var out struct {
		IamToken string `json:"iamToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	iamToken = out.IamToken
	log.Printf("IAM token: %s", out.IamToken)
	iamExp = time.Now().Add(11 * time.Hour)
	return nil
}

func yandexOCR(ctx context.Context, image []byte, langs []string) (string, error) {
	// 1) IAM токен (как у тебя реализовано в ensureIAM)
	if err := ensureIAM(ctx); err != nil {
		log.Printf("failed to ensure IAM: %v", err)
		return "", err
	}

	// 2) Собираем тело запроса для нового OCR endpoint
	reqBody := ocrRecognizeRequest{
		Content:       base64.StdEncoding.EncodeToString(image),
		MimeType:      sniffMime(image),
		LanguageCodes: langs,
		Model:         "page",
	}
	log.Printf("mimetype: %s", reqBody.MimeType)

	payload, _ := json.Marshal(reqBody)
	// 3) Запрос
	url := "https://ocr.api.cloud.yandex.net/ocr/v1/recognizeText"
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Api-Key "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-folder-id", folderID)

	resp, err := httpc.Do(req)
	if err != nil {
		log.Printf("failed to send ocr request: %s", err)
		return "", err
	}
	defer resp.Body.Close()

	// 4) Если 401 — пробуем обновить IAM и повторить один раз
	if resp.StatusCode == http.StatusUnauthorized {
		log.Printf("response status: %s", resp.Status)
		iamToken = ""
		if err := ensureIAM(ctx); err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+iamToken)
		resp, err = httpc.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
	}
	log.Printf("response status: %s", resp.Status)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ocr %d: %s", resp.StatusCode, string(b))
	}

	// 5) Разбор ответа
	var out ocrRecognizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}

	ta := out.TextAnnotation
	if ta == nil {
		log.Printf("ocr recognize: no textAnnotation")
		return "", nil
	}

	// 6) Приоритет — fullText
	if t := strings.TrimSpace(ta.FullText); t != "" {
		log.Printf("ocr recognize: textAnnotation: %s", t)
		return t, nil
	}

	// 7) Фоллбэк — собрать строки из blocks.lines[].text
	var lines []string
	for _, b := range ta.Blocks {
		for _, l := range b.Lines {
			if s := strings.TrimSpace(l.Text); s != "" {
				lines = append(lines, s)
			}
		}
	}
	if len(lines) > 0 {
		return strings.Join(lines, "\n"), nil
	}

	return "", nil
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:16]
}

func sniffMime(b []byte) string {
	// JPEG
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xD8 {
		return "image/jpeg"
	}
	// PNG
	if len(b) >= 8 &&
		b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47 &&
		b[4] == 0x0D && b[5] == 0x0A && b[6] == 0x1A && b[7] == 0x0A {
		return "image/png"
	}
	// PDF (magic: %PDF-)
	if len(b) >= 5 && b[0] == '%' && b[1] == 'P' && b[2] == 'D' && b[3] == 'F' && b[4] == '-' {
		return "application/pdf"
	}
	return "" // можно не указывать — но лучше проставлять
}
