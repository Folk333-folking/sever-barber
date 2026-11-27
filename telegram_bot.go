package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// --- Telegram API types (minimal) ---

type tgUpdate struct {
	UpdateID      int64       `json:"update_id"`
	Message       *tgMessage  `json:"message"`
	CallbackQuery *tgCallback `json:"callback_query"`
}

type tgMessage struct {
	MessageID int64   `json:"message_id"`
	From      *tgUser `json:"from"`
	Chat      tgChat  `json:"chat"`
	Text      string  `json:"text"`
}

type tgCallback struct {
	ID      string     `json:"id"`
	From    *tgUser    `json:"from"`
	Message *tgMessage `json:"message"`
	Data    string     `json:"data"`
}

type tgUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type tgChat struct {
	ID int64 `json:"id"`
}

type tgInlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type tgInlineKeyboard struct {
	InlineKeyboard [][]tgInlineButton `json:"inline_keyboard"`
}

// --- Session state ---

type tgStep int

const (
	stepIdle tgStep = iota
	stepMaster
	stepService
	stepDate
	stepTime
	stepName
	stepPhone
)

type tgSession struct {
	Step      tgStep
	MasterID  string
	ServiceID string
	Date      string
	Time      string
	Name      string
	Phone     string
}

var (
	tgSessions   = map[int64]*tgSession{}
	tgSessionsMu sync.Mutex
)

func getSession(chatID int64) *tgSession {
	tgSessionsMu.Lock()
	defer tgSessionsMu.Unlock()
	s, ok := tgSessions[chatID]
	if !ok {
		s = &tgSession{Step: stepIdle}
		tgSessions[chatID] = s
	}
	return s
}

func clearSession(chatID int64) {
	tgSessionsMu.Lock()
	defer tgSessionsMu.Unlock()
	delete(tgSessions, chatID)
}

// --- HTTP handler ---

func handleTelegramWebhook(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad body", 400)
			return
		}
		var upd tgUpdate
		if err := json.Unmarshal(body, &upd); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		w.WriteHeader(200)
		go processUpdate(app, &upd)
	}
}

func processUpdate(app *App, upd *tgUpdate) {
	if upd.CallbackQuery != nil {
		handleCallback(app, upd.CallbackQuery)
		return
	}
	if upd.Message == nil || upd.Message.Text == "" {
		return
	}
	chatID := upd.Message.Chat.ID
	text := strings.TrimSpace(upd.Message.Text)

	if text == "/start" || text == "/book" || text == "Записаться" {
		startBooking(chatID)
		return
	}
	if text == "/cancel" || text == "Отмена" {
		clearSession(chatID)
		tgSend(chatID, "Запись отменена. Напиши /start чтобы начать снова.", nil)
		return
	}

	s := getSession(chatID)
	switch s.Step {
	case stepName:
		s.Name = text
		s.Step = stepPhone
		tgSend(chatID, "И телефон (можно +7…):", nil)
	case stepPhone:
		s.Phone = text
		finishBooking(app, chatID, s)
	default:
		tgSend(chatID, "Нажми /start чтобы записаться, или /cancel чтобы отменить.", nil)
	}
}

func startBooking(chatID int64) {
	clearSession(chatID)
	s := getSession(chatID)
	s.Step = stepMaster

	var rows [][]tgInlineButton
	for _, m := range masters {
		rows = append(rows, []tgInlineButton{{
			Text:         m.Name + " · " + m.Role,
			CallbackData: "master:" + m.ID,
		}})
	}
	kb := &tgInlineKeyboard{InlineKeyboard: rows}
	tgSend(chatID, "✂️ <b>СЕВЕР</b> — запись\n\nВыбери мастера:", kb)
}

func handleCallback(app *App, cb *tgCallback) {
	chatID := cb.Message.Chat.ID
	data := cb.Data
	tgAnswerCallback(cb.ID)

	s := getSession(chatID)
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		return
	}
	kind, val := parts[0], parts[1]

	switch kind {
	case "master":
		s.MasterID = val
		s.Step = stepService
		var rows [][]tgInlineButton
		for _, svc := range services {
			rows = append(rows, []tgInlineButton{{
				Text:         fmt.Sprintf("%s · %d ₽", svc.Name, svc.Price),
				CallbackData: "service:" + svc.ID,
			}})
		}
		tgSend(chatID, "Отлично. Теперь услуга:", &tgInlineKeyboard{InlineKeyboard: rows})

	case "service":
		s.ServiceID = val
		s.Step = stepDate
		var rows [][]tgInlineButton
		now := time.Now()
		for i := 0; i < 7; i++ {
			d := now.AddDate(0, 0, i)
			label := formatRuDate(d.Format("2006-01-02"))
			if i == 0 {
				label = "Сегодня"
			} else if i == 1 {
				label = "Завтра"
			}
			rows = append(rows, []tgInlineButton{{
				Text:         label,
				CallbackData: "date:" + d.Format("2006-01-02"),
			}})
		}
		tgSend(chatID, "Выбери день:", &tgInlineKeyboard{InlineKeyboard: rows})

	case "date":
		s.Date = val
		s.Step = stepTime
		slots := generateSlots(val)
		var free []string
		for _, t := range slots {
			if !app.IsTaken(s.MasterID, val, t) {
				free = append(free, t)
			}
		}
		if len(free) == 0 {
			tgSend(chatID, "На этот день свободных слотов нет. Выбери другой день (/start).", nil)
			s.Step = stepDate
			return
		}
		var rows [][]tgInlineButton
		row := []tgInlineButton{}
		for i, t := range free {
			row = append(row, tgInlineButton{Text: t, CallbackData: "time:" + t})
			if len(row) == 3 || i == len(free)-1 {
				rows = append(rows, row)
				row = []tgInlineButton{}
			}
		}
		tgSend(chatID, "Свободное время:", &tgInlineKeyboard{InlineKeyboard: rows})

	case "time":
		s.Time = val
		s.Step = stepName
		tgSend(chatID, fmt.Sprintf(
			"Итого:\n• %s\n• %s\n• %s в %s\n\nКак тебя зовут?",
			findMasterName(s.MasterID),
			findServiceName(s.ServiceID),
			formatRuDate(s.Date),
			s.Time,
		), nil)
	}
}

func finishBooking(app *App, chatID int64, s *tgSession) {
	b := Booking{
		MasterID:  s.MasterID,
		ServiceID: s.ServiceID,
		Date:      s.Date,
		Time:      s.Time,
		Name:      s.Name,
		Phone:     s.Phone,
	}
	if err := app.Add(b); err != nil {
		tgSend(chatID, "Это время только что заняли. Начни заново: /start", nil)
		clearSession(chatID)
		return
	}
	go notifyTelegram(b)

	tgSend(chatID, fmt.Sprintf(
		"✅ <b>Запись подтверждена</b>\n\n"+
			"%s\n%s · %s\nмастер %s\n\n"+
			"Ждём тебя. Отменить: напиши нам или /start для новой записи.\n\n"+
			"<i>id</i> <code>%s</code>",
		formatRuDate(b.Date)+" · "+b.Time,
		findServiceName(b.ServiceID),
		b.Name,
		findMasterName(b.MasterID),
		b.ID,
	), nil)
	clearSession(chatID)
}

// --- low-level API ---

func tgAPI(method string, payload any) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return
	}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", token, method)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Println("tg api:", method, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		log.Println("tg api error:", method, resp.StatusCode, string(b))
	}
}

func tgSend(chatID int64, text string, kb *tgInlineKeyboard) {
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if kb != nil {
		payload["reply_markup"] = kb
	}
	tgAPI("sendMessage", payload)
}

func tgAnswerCallback(callbackID string) {
	tgAPI("answerCallbackQuery", map[string]any{"callback_query_id": callbackID})
}

func setupTelegramWebhook() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return
	}
	base := os.Getenv("WEBHOOK_BASE_URL")
	if base == "" {
		base = os.Getenv("RENDER_EXTERNAL_URL")
	}
	if base == "" {
		log.Println("Telegram webhook: set WEBHOOK_BASE_URL to enable bot booking")
		return
	}
	base = strings.TrimRight(base, "/")
	hook := base + "/telegram/webhook"
	api := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", token)
	resp, err := http.PostForm(api, map[string][]string{"url": {hook}})
	if err != nil {
		log.Println("setWebhook:", err)
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	log.Println("Telegram webhook:", hook, string(b))
}
