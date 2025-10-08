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
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (r *Router) acceptPhoto(msg tgbotapi.Message, engines Engines) {
	cid := msg.Chat.ID
	ph := msg.Photo[len(msg.Photo)-1]
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

	key := "chat:" + fmt.Sprint(cid)
	if msg.MediaGroupID != "" {
		key = "grp:" + msg.MediaGroupID
	}

	bi, _ := batches.LoadOrStore(key, &photoBatch{
		ChatID: cid, Key: key, MediaGroupID: msg.MediaGroupID, images: make([][]byte, 0, 4),
	})
	b := bi.(*photoBatch)

	b.mu.Lock()
	b.images = append(b.images, imgBytes)
	if b.timer != nil {
		b.timer.Stop()
	}
	b.timer = time.AfterFunc(debounce, func() { r.processBatch(key, engines) })
	b.mu.Unlock()

	if len(b.images) == 1 {
		r.send(cid, r.PhotoAcceptedText())
	}
}

func (r *Router) processBatch(key string, engines Engines) {
	ctx := context.Background()
	bi, ok := batches.Load(key)
	if !ok {
		return
	}
	b := bi.(*photoBatch)

	b.mu.Lock()
	images := append([][]byte(nil), b.images...)
	chatID := b.ChatID
	mediaGroupID := b.MediaGroupID
	batches.Delete(key)
	b.mu.Unlock()

	if len(images) == 0 {
		return
	}

	merged, err := combineAsOne(images)
	if err != nil {
		r.SendError(chatID, fmt.Errorf("склейка: %w", err))
		return
	}

	r.runDetectThenParse(ctx, chatID, merged, mediaGroupID, engines)
}

func combineAsOne(images [][]byte) ([]byte, error) {
	decoded := make([]image.Image, 0, len(images))
	widths := make([]int, 0, len(images))
	heights := make([]int, 0, len(images))

	for _, b := range images {
		img, _, err := image.Decode(bytes.NewReader(b))
		if err != nil {
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
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)

	y := 0
	for i, img := range decoded {
		w := widths[i]
		h := heights[i]
		x := (maxW - w) / 2
		rect := image.Rect(x, y, x+w, y+h)
		draw.Draw(dst, rect, img, img.Bounds().Min, draw.Over)
		y += h
	}

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

	var out bytes.Buffer
	if err := jpeg.Encode(&out, final, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func tryDecodeStrict(b []byte) (image.Image, error) {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xD8 {
		return jpeg.Decode(bytes.NewReader(b))
	}
	if len(b) >= 8 &&
		b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47 &&
		b[4] == 0x0D && b[5] == 0x0A && b[6] == 0x1A && b[7] == 0x0A {
		return png.Decode(bytes.NewReader(b))
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

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
