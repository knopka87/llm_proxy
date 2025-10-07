package telegram

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"child-bot/api/internal/ocr"
	"child-bot/api/internal/ocr/deepseek"
	"child-bot/api/internal/ocr/gemini"
	"child-bot/api/internal/ocr/openai"
	"child-bot/api/internal/ocr/yandex"
	"child-bot/api/internal/util"
)

// Engines — конкретные реализации движков инжектятся из main.go
type Engines struct {
	Yandex   *yandex.Engine
	Gemini   *gemini.Engine
	OpenAI   *openai.Engine
	Deepseek *deepseek.Engine
}

// ====== Aggregation of multiple photos ======

const debounce = 1200 * time.Millisecond
const maxPixels = 18000000 // ниже лимита 20Мп для запаса

type photoBatch struct {
	ChatID       int64
	Key          string // "grp:<mediaGroupID>" | "chat:<chatID>"
	MediaGroupID string

	mu     sync.Mutex
	images [][]byte
	timer  *time.Timer
	lastAt time.Time
}

var batches sync.Map       // key string -> *photoBatch
var pendingChoice sync.Map // chatID -> []string (tasks brief)

// HandleUpdate — главный обработчик апдейтов
func (r *Router) HandleUpdate(upd tgbotapi.Update, engines Engines) {
	if upd.Message == nil {
		return
	}
	cid := upd.Message.Chat.ID

	// ====== Обработка выбора номера, когда детектор нашёл несколько заданий ======
	if v, ok := pendingChoice.Load(cid); ok && upd.Message.Text != "" {
		briefs := v.([]string)
		txt := strings.TrimSpace(upd.Message.Text)
		if n, err := strconv.Atoi(txt); err == nil && n >= 1 && n <= len(briefs) {
			pendingChoice.Delete(cid)
			r.send(cid, fmt.Sprintf("Ок, беру задание: %s\nПришли, пожалуйста, фото ещё раз для обработки.", briefs[n-1]))
			return
		}
		// если не число или вне диапазона — молча игнорируем и ждём корректного номера
	}

	// ====== Команды ======
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
				r.send(cid, "✅ Движок: yandex (OCR)")
			case "gemini":
				if mdl != "" {
					engines.Gemini.Model = mdl
				}
				r.EngManager.Set(cid, engines.Gemini)
				r.send(cid, "✅ Движок: gemini ("+engines.Gemini.Model+")")
			case "gpt":
				if mdl != "" {
					engines.OpenAI.Model = mdl
				}
				r.EngManager.Set(cid, engines.OpenAI)
				r.send(cid, "✅ Движок: gpt ("+engines.OpenAI.Model+")")
			case "deepseek":
				if mdl != "" {
					engines.Deepseek.Model = mdl
				}
				r.EngManager.Set(cid, engines.Deepseek)
				r.send(cid, "⚠️ Внимание: DeepSeek Chat API не умеет анализировать изображения. Для этого используйте /engine yandex | gemini | gpt.")
			default:
				r.send(cid, "Неизвестный движок. Доступны: yandex | gemini | gpt | deepseek")
			}
			return
		}
		// прочие команды обрабатывает Router.HandleCommand
		r.HandleCommand(upd)
		return
	}

	// ====== ФОТО (поддержка альбомов и серии фото) ======
	if len(upd.Message.Photo) > 0 {
		// Скачиваем самое большое превью
		ph := upd.Message.Photo[len(upd.Message.Photo)-1]
		file, err := r.Bot.GetFile(tgbotapi.FileConfig{FileID: ph.FileID})
		if err != nil {
			r.SendError(cid, err)
			return
		}
		url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", r.Bot.Token, file.FilePath)
		imgBytes, err := download(url)
		if err != nil {
			r.SendError(cid, err)
			return
		}

		// Определяем ключ пачки: альбом (media_group) или серия сообщений по чату
		key := ""
		if upd.Message.MediaGroupID != "" {
			key = "grp:" + upd.Message.MediaGroupID
		} else {
			key = fmt.Sprintf("chat:%d", cid)
		}

		// Берём/создаём пачку
		bi, _ := batches.LoadOrStore(key, &photoBatch{
			ChatID:       cid,
			Key:          key,
			MediaGroupID: upd.Message.MediaGroupID,
			images:       make([][]byte, 0, 4),
		})
		b := bi.(*photoBatch)

		// Добавляем фото и перезапускаем таймер
		b.mu.Lock()
		b.images = append(b.images, imgBytes)
		b.lastAt = time.Now()
		if b.timer != nil {
			b.timer.Stop()
		}
		b.timer = time.AfterFunc(debounce, func() {
			r.processBatch(key, engines)
		})
		b.mu.Unlock()

		// Сообщение пользователю показываем один раз — на первое фото
		if len(b.images) == 1 {
			r.send(cid, r.PhotoAcceptedText())
		}
	}
}

// processBatch извлекает пачку, склеивает и передаёт в детектор, затем — в выбранный движок
func (r *Router) processBatch(key string, engines Engines) {
	bi, ok := batches.Load(key)
	if !ok {
		return
	}
	b := bi.(*photoBatch)

	// Копируем и очищаем пачку
	b.mu.Lock()
	images := make([][]byte, len(b.images))
	copy(images, b.images)
	chatID := b.ChatID
	batches.Delete(key)
	b.mu.Unlock()

	if len(images) == 0 {
		return
	}

	// Склейка изображений в одно (вертикально)
	merged, err := combineAsOne(images)
	if err != nil {
		r.SendError(chatID, fmt.Errorf("склейка изображений: %w", err))
		return
	}

	// --- DETECT stage (PROMPT_DETECT) ---
	mime := util.SniffMimeHTTP(merged)
	var dres ocr.DetectResult
	var derr error

	// Предпочитаем детектор Gemini, иначе OpenAI; если ключей нет — детектор пропускаем
	if engines.Gemini != nil && engines.Gemini.APIKey != "" {
		dres, derr = engines.Gemini.Detect(context.Background(), merged, mime, 0)
	} else if engines.OpenAI != nil && engines.OpenAI.APIKey != "" {
		dres, derr = engines.OpenAI.Detect(context.Background(), merged, mime, 0)
	}
	if derr != nil {
		// Детектор не обязателен: информируем и продолжаем
		r.send(chatID, "⚠️ Не удалось оценить снимок, продолжаю распознавание.")
	} else {
		// Политика из PROMPT_DETECT
		if dres.FinalState == "inappropriate_image" {
			r.send(chatID, "⚠️ Похоже, это неприемлемое изображение. Пожалуйста, пришлите фото учебного задания без личных данных.")
			return
		}
		if dres.NeedsRescan {
			msg := "Пожалуйста, переснимите фото"
			if dres.RescanReason != "" {
				msg += ": " + dres.RescanReason
			}
			r.send(chatID, "📷 "+msg)
			return
		}
		if dres.HasFaces {
			r.send(chatID, "ℹ️ На фото видны лица. Лучше переснять без лиц, чтобы сохранить приватность.")
		}
		if dres.MultipleTasksDetected {
			// Если есть явный лидер и высокая уверенность — не тревожим уточнениями
			if dres.AutoChoiceSuggested && dres.TopCandidateIndex != nil &&
				*dres.TopCandidateIndex >= 0 && *dres.TopCandidateIndex < len(dres.TasksBrief) &&
				dres.Confidence >= 0.80 {
				// продолжаем без уточнений
			} else {
				// Просим выбрать номер задания
				if len(dres.TasksBrief) > 0 {
					pendingChoice.Store(chatID, dres.TasksBrief)
					var bld strings.Builder
					bld.WriteString("Нашёл на фото несколько заданий. Выбери номер:\n")
					for i, t := range dres.TasksBrief {
						fmt.Fprintf(&bld, "%d) %s\n", i+1, t)
					}
					if dres.DisambiguationQuestion != "" {
						bld.WriteString("\n")
						bld.WriteString(dres.DisambiguationQuestion)
					}
					r.send(chatID, bld.String())
					return
				}
			}
		}
	}

	eng := r.EngManager.Get(chatID)

	if eng.Name() == "gemini" || eng.Name() == "gpt" {
		pr, pErr := eng.Parse(context.Background(), merged, 0)
		if pErr == nil {
			// если нужно подтверждение — спрашиваем один раз и ждём ответа
			if pr.ConfirmationNeeded {
				var b strings.Builder
				b.WriteString("Я так прочитал задание. Всё верно?\n")
				if strings.TrimSpace(pr.RawText) != "" {
					b.WriteString("```\n")
					b.WriteString(pr.RawText)
					b.WriteString("\n```\n")
				}
				if strings.TrimSpace(pr.Question) != "" {
					b.WriteString("\nВопрос: ")
					b.WriteString(pr.Question)
					b.WriteString("\n")
				}
				b.WriteString("\nОтветьте: да / нет")
				r.send(chatID, b.String())

				// тут можно сохранить pr в pending map, чтобы на "да" продолжить без повторного парсинга
				// (по желанию)
				// parsePending.Store(chatID, pr)
				// return
			}
			// при автоподтверждении просто продолжаем к Analyze
		} else {
			r.send(chatID, "⚠️ Не удалось чётко переписать задание, продолжаю анализ.")
		}
	}

	// --- Основной анализ выбранным движком ---
	res, err := eng.Analyze(context.Background(), merged, ocr.Options{
		Langs: []string{"ru", "en"},
	})
	if err != nil {
		r.SendError(chatID, err)
		return
	}

	switch eng.Name() {
	case "yandex":
		// Только транскрипт (OCR)
		txt := strings.TrimSpace(res.Text)
		if txt == "" {
			txt = "(пусто)"
		}
		r.SendResult(chatID, txt)
	default:
		// Аналитический ответ (текст задачи / вердикт / 3 подсказки)
		var bld strings.Builder
		if strings.TrimSpace(res.Text) != "" {
			bld.WriteString("📄 *Текст задачи:*\n```\n")
			bld.WriteString(res.Text)
			bld.WriteString("\n```\n\n")
		}
		if res.FoundSolution {
			switch res.SolutionVerdict {
			case "correct":
				bld.WriteString("✅ Задача решена верно.\n\n")
			case "incorrect":
				bld.WriteString("⚠️ В решении есть ошибка.\n")
				if strings.TrimSpace(res.SolutionNote) != "" {
					bld.WriteString("Подсказка где/какого рода: ")
					bld.WriteString(res.SolutionNote)
					bld.WriteString("\n\n")
				} else {
					bld.WriteString("\n")
				}
			default:
				bld.WriteString("ℹ️ Решение обнаружено, но проверка неуверенна.\n\n")
			}
		} else {
			bld.WriteString("ℹ️ На изображении нет готового решения.\n\n")
		}
		if len(res.Hints) > 0 {
			bld.WriteString("💡 *Подсказки (L1→L3):*\n")
			for i, h := range res.Hints {
				fmt.Fprintf(&bld, "%d) %s\n", i+1, h)
			}
		}
		msg := tgbotapi.NewMessage(chatID, bld.String())
		msg.ParseMode = "Markdown"
		_, _ = r.Bot.Send(msg)
	}
}

// ====== Utilities ======

func download(url string) ([]byte, error) {
	resp, err := httpClient().Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return io.ReadAll(resp.Body)
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

// combineAsOne объединяет несколько картинок в одну (вертикально).
// Разная ширина центрируется на белом фоне.
// Если итоговый размер по пикселям > maxPixels — делаем downscale.
func combineAsOne(images [][]byte) ([]byte, error) {
	decoded := make([]image.Image, 0, len(images))
	widths := make([]int, 0, len(images))
	heights := make([]int, 0, len(images))

	for _, b := range images {
		img, _, err := image.Decode(bytes.NewReader(b))
		if err != nil {
			// попробуем определить по форматам напрямую
			if try, err2 := tryDecodeStrict(b); err2 == nil {
				img = try
			} else {
				return nil, err
			}
		}
		decoded = append(decoded, img)
		bounds := img.Bounds()
		widths = append(widths, bounds.Dx())
		heights = append(heights, bounds.Dy())
	}

	// вычисляем финальные размеры
	maxW := 0
	sumH := 0
	for i := range decoded {
		if widths[i] > maxW {
			maxW = widths[i]
		}
		sumH += heights[i]
	}
	if maxW == 0 || sumH == 0 {
		return nil, fmt.Errorf("пустые изображения")
	}

	dst := image.NewRGBA(image.Rect(0, 0, maxW, sumH))
	// фон — белый
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)

	// рендерим по очереди, выравнивание по центру
	y := 0
	for i, img := range decoded {
		w := widths[i]
		h := heights[i]
		x := (maxW - w) / 2
		rect := image.Rect(x, y, x+w, y+h)
		draw.Draw(dst, rect, img, img.Bounds().Min, draw.Over)
		y += h
	}

	// downscale при превышении лимита пикселей
	totalPx := maxW * sumH
	final := image.Image(dst)
	if totalPx > maxPixels {
		scale := math.Sqrt(float64(maxPixels) / float64(totalPx))
		newW := int(float64(maxW)*scale + 0.5)
		newH := int(float64(sumH)*scale + 0.5)
		if newW < 1 {
			newW = 1
		}
		if newH < 1 {
			newH = 1
		}
		final = scaleDownNN(dst, newW, newH)
	}

	// Кодируем в JPEG (качество 90)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, final, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// tryDecodeStrict — пробуем строго PNG/JPEG, иначе стандартный Decode
func tryDecodeStrict(b []byte) (image.Image, error) {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xD8 {
		return jpeg.Decode(bytes.NewReader(b))
	}
	if len(b) >= 8 &&
		b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47 &&
		b[4] == 0x0D && b[5] == 0x0A && b[6] == 0x1A && b[7] == 0x0A {
		return png.Decode(bytes.NewReader(b))
	}
	// по умолчанию — std Decode ещё раз
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

// Простейший nearest-neighbor даунскейл (без внешних зависимостей)
func scaleDownNN(src image.Image, newW, newH int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	sb := src.Bounds()
	srcW := sb.Dx()
	srcH := sb.Dy()
	for y := 0; y < newH; y++ {
		sy := sb.Min.Y + (y*srcH)/newH
		for x := 0; x < newW; x++ {
			sx := sb.Min.X + (x*srcW)/newW
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}
