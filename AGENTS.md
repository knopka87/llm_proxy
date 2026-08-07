# llm_proxy — руководство для AI-агента

Go HTTP-микросервис: единая прокси между `child_bot` и LLM-провайдерами
(**OpenAI**, **Google Gemini**, **OpenRouter**). Принимает фото учебной задачи и делает
4 шага: `detect` (качество фото + классификация предмета) → `parse` (OCR + структура задачи)
→ `hint` (подсказки L1/L2/L3) → `check_solution` (проверка ответа ученика).

Парный проект — `../bot` (приложение child_bot; см. `../CLAUDE.md`), который ходит сюда по HTTP.
Никаких обратных вызовов из прокси нет.

## Карта репозитория

Без фреймворков — чистый `net/http.ServeMux`. Есть **v1 (legacy, устаревший)** и **v2 (актуальный)**.
Новую работу веди в `v2/`.

| Путь | Что там |
|------|---------|
| `api/cmd/llm-proxy/main.go` | **Точка входа.** Инициализация движков, регистрация `/v1/*` и `/v2/*`, старт сервера |
| `api/internal/config/config.go` | Загрузка env (`Load()`); `GEMINI_API_KEY` и `OPENAI_API_KEY` обязательны |
| `api/internal/v2/handle/` | **HTTP-хендлеры** v2 (detect, parse, hint, check_solution, analogue, *_ru) |
| `api/internal/v2/ocr/engine.go` | **Интерфейс `Engine`** — контракт всех провайдеров |
| `api/internal/v2/ocr/gpt/` | Клиент OpenAI (Responses API `/v1/responses`); шаги в отдельных файлах |
| `api/internal/v2/ocr/gemini/` | Клиент Gemini (лучший OCR рукописи) |
| `api/internal/v2/ocr/openrouter/` | Клиент OpenRouter (Chat Completions API) |
| `api/internal/v2/ocr/mixed/engine.go` | Роутер: detect+parse → Gemini, hint+check → OpenAI (hardcoded) |
| `api/internal/v2/ocr/types/` | Go-структуры request/response + `stats.go` (токены/латентность) |
| `api/internal/v2/tmplrouter/router.go` | Выбор педагогического шаблона T1–T52 по содержимому задачи |
| `api/internal/v2/templates/math/` | Шаблоны `T1.json`…`T52.json` |
| `api/internal/v2/prompt/`, `.../gpt/prompt/` | JSON-схемы и промпты (`*.system.txt`, `*.user.txt`) |
| `api/internal/v1/` | Legacy-версия API — трогать только для совместимости |
| `docs/llm_analysis.md` | **Ключевой документ:** модели, цены, TTFB, стратегии оптимизации |

## Выбор провайдера

Клиент передаёт поле `llm_name` в JSON тела запроса: `"gpt"` | `"gemini"` | `"mixed"` | `"openrouter"`.
Таймаут — заголовок `X-Request-Timeout` или `?timeoutSec=NN` (дефолт 180с, максимум 5 мин).
Ответ несёт метрики в заголовках `X-LLM-Input-Tokens`, `X-LLM-Output-Tokens`, `X-LLM-Latency-Ms`,
`X-LLM-Model` — их логирует `child_bot`.

## Команды

Makefile нет — используй `go`/`docker` напрямую. Go-версия 1.26.2.

```bash
go build -o server ./api/cmd/llm-proxy    # сборка
go test ./...                             # тесты (покрытие минимальное)
GEMINI_API_KEY=... OPENAI_API_KEY=... LLM_PROXY_ALLOWED_CLIENT_CIDRS=127.0.0.1/32 PORT=8000 ./server   # запуск, сервер на :8000
docker compose -f llm-proxy.compose.yml up --build         # через Docker
```

Health-check: `GET /healthz`.

## Конфигурация

Читается в `api/internal/config/config.go`. Шаблон — `.env.example` (`.env` не коммитить).

- **Обязательны:** `GEMINI_API_KEY`, `OPENAI_API_KEY`.
- **Опционально:** `OPENROUTER_API_KEY` — если пусто, `llm_name="openrouter"` вернёт ошибку 502.
- Модели по шагам: `GEMINI_DETECT_MODEL`, `GEMINI_PARSE_MODEL`, `OPENAI_MODEL`,
  `OPENROUTER_{DETECT,PARSE,HINT,CHECK}_MODEL` (дефолты — в `llm-proxy.compose.yml`).
- `PORT` (8000), `PROMPT_DIR` (переопределяет встроенные промпты).
- `LLM_PROXY_ALLOWED_CLIENT_CIDRS` обязателен; `LLM_PROXY_TRUSTED_PROXY_CIDRS`
  задаётся только для reverse proxy, который перезаписывает `X-Forwarded-For`.

## Подводные камни

- **Промпты компилируются в бинарник.** Правка `.txt`/`.json` требует пересборки, либо запуска
  с `PROMPT_DIR`, указывающим на изменённые файлы.
- Поле `template` в hint-запросе **устарело** и игнорируется — шаблон выбирает `tmplrouter`.
  Его логика портирована из `../bot/api/internal/llm/template_router.go` (держать синхронно).
- **Нет авторизации и rate-limiting** — защита ожидается на уровне nginx/ingress.
- Два стиля API: v1 = OpenAI Responses (`input[]`), v2 = Chat Completions (`messages[]`).
- `mixed`-стратегия захардкожена в `mixed/engine.go`.
- Оптимизации моделей из `docs/llm_analysis.md` (например замена `gpt-5-mini` в hint/check) в коде
  ещё **не применены** — сверяйся с документом перед изменением моделей.

## Стиль (Go)

Идиоматичный Go: явные ошибки с `%w`, `context.Context` первым параметром, интерфейс `Engine`
на стороне потребителя. `gofmt` + `go vet` перед коммитом. Коммиты — на английском.
