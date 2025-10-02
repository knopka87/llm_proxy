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

	httpc    = &http.Client{Timeout: 60 * time.Second}
	iamToken string
	iamExp   time.Time
)

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
	iamExp = time.Now().Add(11 * time.Hour)
	return nil
}

func yandexOCR(ctx context.Context, image []byte, langs []string) (string, error) {
	if err := ensureIAM(ctx); err != nil {
		return "", err
	}
	payload := map[string]any{
		"folderId": folderID,
		"analyze_specs": []any{
			map[string]any{
				"content": base64.StdEncoding.EncodeToString(image),
				"features": []any{
					map[string]any{
						"type": "TEXT_DETECTION",
						"text_detection_config": map[string]any{
							"language_codes": langs,
						},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://vision.api.cloud.yandex.net/vision/v1/batchAnalyze",
		bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+iamToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// одна попытка обновить IAM при 401
	if resp.StatusCode == 401 {
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
	if resp.StatusCode != 200 {
		x, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("vision %d: %s", resp.StatusCode, string(x))
	}

	// Разбор 2 популярных схем ответа
	var vr struct {
		Results []struct {
			Results []struct {
				TextAnnotation *struct {
					Text string `json:"text"`
				} `json:"textAnnotation,omitempty"`
				TextDetection *struct {
					FullTextAnnotation *struct {
						Text string `json:"text"`
					} `json:"fullTextAnnotation,omitempty"`
				} `json:"textDetection,omitempty"`
			} `json:"results"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return "", err
	}
	for _, r1 := range vr.Results {
		for _, r2 := range r1.Results {
			if r2.TextAnnotation != nil && r2.TextAnnotation.Text != "" {
				return r2.TextAnnotation.Text, nil
			}
			if r2.TextDetection != nil && r2.TextDetection.FullTextAnnotation != nil &&
				r2.TextDetection.FullTextAnnotation.Text != "" {
				return r2.TextDetection.FullTextAnnotation.Text, nil
			}
		}
	}
	return "", nil
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:16]
}
