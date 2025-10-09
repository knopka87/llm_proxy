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

// Три кнопки действий после подсказки/парсинга
func makeActionsKeyboard() tgbotapi.InlineKeyboardMarkup {
	btnHint := tgbotapi.NewInlineKeyboardButtonData("Показать подсказку", "hint_next")
	btnReady := tgbotapi.NewInlineKeyboardButtonData("Готов дать решение", "ready_solution")
	btnNew := tgbotapi.NewInlineKeyboardButtonData("Перейти к новой задаче", "new_task")
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(btnHint),
		tgbotapi.NewInlineKeyboardRow(btnReady),
		tgbotapi.NewInlineKeyboardRow(btnNew),
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
