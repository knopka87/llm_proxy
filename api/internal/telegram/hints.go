package telegram

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"child-bot/api/internal/ocr"
	"child-bot/api/internal/util"
)

type hintSession struct {
	Image        []byte
	Mime         string
	MediaGroupID string
	Parse        ocr.ParseResult
	Detect       ocr.DetectResult
	EngineName   string
	Model        string
	NextLevel    int
}

func (r *Router) showTaskAndPrepareHints(chatID int64, sc *selectionContext, pr ocr.ParseResult, llm ocr.Engine) {
	var b strings.Builder
	b.WriteString("📄 *Текст задания:*\n```\n")
	if strings.TrimSpace(pr.RawText) != "" {
		b.WriteString(pr.RawText)
	} else {
		b.WriteString("(не удалось чётко переписать текст)")
	}
	b.WriteString("\n```\n")
	if q := strings.TrimSpace(pr.Question); q != "" {
		b.WriteString("\n*Вопрос:* " + q + "\n")
	}

	msg := tgbotapi.NewMessage(chatID, b.String())
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = makeHintKeyboard()
	_, _ = r.Bot.Send(msg)

	hs := &hintSession{
		Image: sc.Image, Mime: sc.Mime, MediaGroupID: sc.MediaGroupID,
		Parse: pr, Detect: sc.Detect, EngineName: llm.Name(), Model: llm.GetModel(), NextLevel: 1,
	}
	hintState.Store(chatID, hs)
}

func (r *Router) applyTextCorrectionThenShowHints(chatID int64, corrected string, engines Engines) {
	v, ok := parseWait.Load(chatID)
	if !ok {
		return
	}
	p := v.(*parsePending)
	parseWait.Delete(chatID)

	llm := r.resolveEngineByName(p.LLM, engines)
	if llm == nil {
		r.send(chatID, "LLM-движок недоступен.")
		return
	}
	imgHash := util.SHA256Hex(p.Sc.Image)

	pr := p.PR
	pr.RawText = corrected
	pr.ConfirmationNeeded = false
	pr.ConfirmationReason = "user_fix"

	_ = r.ParseRepo.Upsert(context.Background(), chatID, p.Sc.MediaGroupID, imgHash, llm.Name(), llm.GetModel(), pr, true, "user_fix")
	r.showTaskAndPrepareHints(chatID, &selectionContext{
		Image: p.Sc.Image, Mime: p.Sc.Mime, MediaGroupID: p.Sc.MediaGroupID, Detect: p.Sc.Detect,
	}, pr, llm)
}

func formatHint(level int, hr ocr.HintResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "💡 *Подсказка L%d*: %s\n", level, safe(hr.HintTitle))
	for _, s := range hr.HintSteps {
		if t := strings.TrimSpace(s); t != "" {
			fmt.Fprintf(&b, "• %s\n", safe(t))
		}
	}
	if t := strings.TrimSpace(hr.ControlQuestion); t != "" {
		fmt.Fprintf(&b, "\n*Проверь себя:* %s\n", safe(t))
	}
	// Дополнительные поля (при наличии)
	if hr.RuleHint != "" {
		fmt.Fprintf(&b, "_Подсказка по правилу:_ %s\n", safe(hr.RuleHint))
	}
	msg := tgbotapi.NewMessage(0, "") // заглушка для ParseMode
	_ = msg                           // просто, чтобы напомнить: используйте Markdown, поэтому экранируем
	return markdown(b.String())
}

func safe(s string) string {
	// лёгкая защита от Markdown-вставок
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "_", "\\_")
	s = strings.ReplaceAll(s, "*", "\\*")
	s = strings.ReplaceAll(s, "[", "\\[")
	return s
}

func markdown(s string) string {
	// Возвращаем как есть — в месте отправки задаём ParseMode=Markdown при необходимости
	return s
}
