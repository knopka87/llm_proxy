package telegram

import (
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Кнопки подтверждения PARSE
func makeParseConfirmKeyboard() tgbotapi.InlineKeyboardMarkup {
	yes := tgbotapi.NewInlineKeyboardButtonData("Да", "parse_yes")
	no := tgbotapi.NewInlineKeyboardButtonData("Нет", "parse_no")
	return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(yes, no))
}

// Кнопка для поэтапной выдачи L1→L2→L3
func makeHintKeyboard() tgbotapi.InlineKeyboardMarkup {
	btn := tgbotapi.NewInlineKeyboardButtonData("Получить подсказку", "hint_next")
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(btn),
	)
}

// лёгкое экранирование для Markdown (если функции ещё нет)
func esc(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "_", "\\_")
	s = strings.ReplaceAll(s, "*", "\\*")
	s = strings.ReplaceAll(s, "[", "\\[")
	return s
}
