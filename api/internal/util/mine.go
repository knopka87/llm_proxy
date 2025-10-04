package util

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
