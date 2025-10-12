package util

import (
	"encoding/base64"
	"net/http"
	"strings"
)

func SniffMimeForOCR(b []byte) string {
	// JPEG: FF D8
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xD8 {
		return "JPEG"
	}
	// PNG
	if len(b) >= 8 &&
		b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47 &&
		b[4] == 0x0D && b[5] == 0x0A && b[6] == 0x1A && b[7] == 0x0A {
		return "PNG"
	}
	// PDF
	if len(b) >= 5 && b[0] == '%' && b[1] == 'P' && b[2] == 'D' && b[3] == 'F' && b[4] == '-' {
		return "PDF"
	}
	return ""
}

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
	if exp := strings.TrimSpace(explicit); exp != "" {
		return exp
	}
	if h := strings.TrimSpace(hint); h != "" {
		return h
	}
	if len(data) > 0 {
		return http.DetectContentType(data) // вернёт image/jpeg|png|webp|application/pdf и т.д.
	}

	return "image/jpeg"
}
