package telegram

import (
	"sync"
	"time"
)

const (
	debounce  = 1200 * time.Millisecond
	maxPixels = 18_000_000
)

var chatMode sync.Map // chatID -> string: "", "await_solution", "await_new_task"

// хелперы
func setMode(chatID int64, mode string) { chatMode.Store(chatID, mode) }
func getMode(chatID int64) string {
	if v, ok := chatMode.Load(chatID); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return ""
}
func clearMode(chatID int64) { chatMode.Delete(chatID) }

type photoBatch struct {
	ChatID       int64
	Key          string // "grp:<mediaGroupID>" | "chat:<chatID>"
	MediaGroupID string

	mu     sync.Mutex
	images [][]byte
	timer  *time.Timer
	lastAt time.Time
}

var (
	batches       sync.Map // key -> *photoBatch
	pendingChoice sync.Map // chatID -> []string (tasks brief)
	pendingCtx    sync.Map // chatID -> *selectionContext
	parseWait     sync.Map // chatID -> *parsePending
	hintState     sync.Map // chatID -> *hintSession
)
