package util

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"strings"
)

func SniffMimeHTTP(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xD8 {
		return "image/jpeg"
	}
	if len(b) >= 8 &&
		b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47 &&
		b[4] == 0x0D && b[5] == 0x0A && b[6] == 0x1A && b[7] == 0x0A {
		return "image/png"
	}
	return "application/octet-stream"
}

func MakeDataURL(mime, b64 string) string {
	return "data:" + mime + ";base64," + b64
}

// DecodeBase64MaybeDataURL декодирует base64. Если это data:URI, вернёт MIME из префикса.
func DecodeBase64MaybeDataURL(s string) ([]byte, string, error) {
	s = strings.TrimSpace(s)
	var hintMIME string
	if strings.HasPrefix(s, "data:") {
		// data:<mime>;base64,<payload>
		if idx := strings.IndexByte(s, ','); idx > 0 {
			meta := s[len("data:"):idx] // "<mime>;base64"
			if semi := strings.IndexByte(meta, ';'); semi >= 0 {
				hintMIME = meta[:semi]
			} else {
				hintMIME = meta
			}
			s = s[idx+1:]
		}
	}
	// Стандартная база64, затем URL-safe — на случай вариаций
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, hintMIME, nil
	} else if b2, err2 := base64.URLEncoding.DecodeString(s); err2 == nil {
		return b2, hintMIME, nil
	} else {
		return nil, "", err
	}
}

// PickMIME берём явный MIME, затем из data:URI, иначе детектим по байтам.
func PickMIME(explicit, hint string, data []byte) string {
	if exp := strings.TrimSpace(explicit); exp != "" && exp != "application/octet-stream" {
		return exp
	}
	if h := strings.TrimSpace(hint); h != "" && h != "application/octet-stream" {
		return h
	}

	if len(data) > 0 {
		mt := http.DetectContentType(data)
		if mt == "application/octet-stream" {
			// Попробуем руками распознать распространённые форматы и HEIC/AVIF
			if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8 {
				return "image/jpeg"
			}
			if len(data) >= 8 &&
				data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 &&
				data[4] == 0x0D && data[5] == 0x0A && data[6] == 0x1A && data[7] == 0x0A {
				return "image/png"
			}
			if heicAvif := sniffHEICorAVIF(data); heicAvif != "" {
				return heicAvif
			}
		}
		return mt
	}
	// Если данных нет, безопасный дефолт (для Gemini лучше не отправлять octet-stream)
	return "image/jpeg"
}

// sniffHEICorAVIF пытается распознать контейнеры ISOBMFF (HEIC/HEIF/AVIF).
// Ищет сигнатуру ftyp и совместимые бренды в первых байтах.
func sniffHEICorAVIF(data []byte) string {
	if len(data) < 12 {
		return ""
	}
	// ISO BMFF: bytes 4..7 == 'ftyp'
	if !bytes.Equal(data[4:8], []byte{'f', 't', 'y', 'p'}) {
		return ""
	}
	// Major brand
	major := string(data[8:12])
	if isHeicBrand(major) {
		return "image/heic"
	}
	if isAvifBrand(major) {
		return "image/avif"
	}
	// Просканируем ещё немного совместимые бренды (по 4 байта)
	// ограничимся первыми 64 байтами для простоты
	limit := len(data)
	if limit > 64 {
		limit = 64
	}
	for i := 16; i+4 <= limit; i += 4 { // пропускаем size(4) + 'ftyp'(4) + major(4) + minor(4)
		b := string(data[i : i+4])
		if isHeicBrand(b) {
			return "image/heic"
		}
		if isAvifBrand(b) {
			return "image/avif"
		}
	}
	return ""
}

func isHeicBrand(b string) bool {
	switch b {
	case "heic", "heix", "hevc", "hevx", "mif1", "msf1", "heis", "hevm":
		return true
	}
	return false
}

func isAvifBrand(b string) bool {
	switch b {
	case "avif", "avis":
		return true
	}
	return false
}
