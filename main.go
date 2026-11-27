package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed templates/*
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

type Service struct {
	ID          string
	Name        string
	Description string
	Price       int
	Duration    int
}

type Master struct {
	ID       string
	Name     string
	Role     string
	Bio      string
	Initials string
}

type Product struct {
	ID          string
	Name        string
	Description string
	Price       int
	Category    string
	Icon        string
}

type Booking struct {
	ID        string
	MasterID  string
	ServiceID string
	Date      string
	Time      string
	Name      string
	Phone     string
	Created   time.Time
}

type App struct {
	mu       sync.RWMutex
	bookings map[string]Booking
	seq      int
}

func NewApp() *App {
	return &App{
		bookings: make(map[string]Booking),
		seq:      1000,
	}
}

func (a *App) key(master, date, t string) string {
	return master + "|" + date + "|" + t
}

func (a *App) IsTaken(master, date, t string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.bookings[a.key(master, date, t)]
	return ok
}

func (a *App) Add(b Booking) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	k := a.key(b.MasterID, b.Date, b.Time)
	if _, ok := a.bookings[k]; ok {
		return fmt.Errorf("slot taken")
	}
	a.seq++
	b.ID = fmt.Sprintf("B%d", a.seq)
	b.Created = time.Now()
	a.bookings[k] = b
	return nil
}

func (a *App) ByPhone(phone string) []Booking {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var list []Booking
	phone = normalizePhone(phone)
	for _, b := range a.bookings {
		if normalizePhone(b.Phone) == phone {
			list = append(list, b)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Date+list[i].Time < list[j].Date+list[j].Time
	})
	return list
}

func normalizePhone(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if strings.HasPrefix(s, "8") && len(s) == 11 {
		s = "7" + s[1:]
	}
	return s
}

var (
	services = []Service{
		{ID: "cut", Name: "Стрижка", Description: "Классическая или современная. Разговор, форма, финиш.", Price: 1800, Duration: 45},
		{ID: "beard", Name: "Борода", Description: "Оформление, контуры, уход. Машинка + бритва.", Price: 1200, Duration: 30},
		{ID: "combo", Name: "Стрижка + борода", Description: "Полный образ за один визит.", Price: 2700, Duration: 70},
		{ID: "fade", Name: "Фейд / скин", Description: "Чистый переход, аккуратная работа у висков.", Price: 2000, Duration: 50},
		{ID: "kids", Name: "Детская стрижка", Description: "До 12 лет. Спокойно и без спешки.", Price: 1400, Duration: 40},
		{ID: "care", Name: "Укладка + уход", Description: "Шампунь, укладка, рекомендации по продуктам.", Price: 900, Duration: 25},
	}

	masters = []Master{
		{ID: "ivan", Name: "Иван", Role: "Старший барбер", Bio: "10 лет. Любит чёткие линии и спокойный разговор.", Initials: "ИВ"},
		{ID: "max", Name: "Максим", Role: "Барбер", Bio: "Фейды, текстура, современный стиль.", Initials: "МК"},
		{ID: "alex", Name: "Алекс", Role: "Барбер", Bio: "Классика и борода. Акцент на форму лица.", Initials: "АЛ"},
	}

	products = []Product{
		{ID: "sh1", Name: "Шампунь ежедневный", Description: "Мягкое очищение без пересушивания.", Price: 790, Category: "Волосы", Icon: "bottle"},
		{ID: "cd1", Name: "Кондиционер", Description: "Облегчает расчёсывание, придаёт блеск.", Price: 820, Category: "Волосы", Icon: "bottle"},
		{ID: "gl1", Name: "Глина матовая", Description: "Сильная фиксация, естественный вид.", Price: 950, Category: "Укладка", Icon: "jar"},
		{ID: "pt1", Name: "Паста текстурная", Description: "Объём и подвижность на весь день.", Price: 890, Category: "Укладка", Icon: "jar"},
		{ID: "bo1", Name: "Масло для бороды", Description: "Питает и смягчает. Древесный аромат.", Price: 1100, Category: "Борода", Icon: "drop"},
		{ID: "bs1", Name: "Бальзам после бритья", Description: "Снимает раздражение, увлажняет.", Price: 720, Category: "Борода", Icon: "tube"},
	}
)

func main() {
	app := NewApp()
	tmpl := template.Must(template.New("").Funcs(template.FuncMap{
		"price": func(n int) string {
			return fmt.Sprintf("%d ₽", n)
		},
		"findService": func(id string) Service {
			for _, s := range services {
				if s.ID == id {
					return s
				}
			}
			return Service{}
		},
		"findMaster": func(id string) Master {
			for _, m := range masters {
				if m.ID == id {
					return m
				}
			}
			return Master{}
		},
	}).ParseFS(templatesFS, "templates/*.html"))

	mux := http.NewServeMux()

	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticContent))))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data := map[string]any{
			"Services":    services,
			"Masters":     masters,
			"Products":    products,
			"Title":       "СЕВЕР — барбершоп",
			"TelegramURL": telegramBotURL(),
		}
		if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
			log.Println(err)
			http.Error(w, "template error", 500)
		}
	})

	mux.HandleFunc("/book", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleBookPost(w, r, app)
			return
		}
		master := r.URL.Query().Get("master")
		service := r.URL.Query().Get("service")
		data := map[string]any{
			"Services":    services,
			"Masters":     masters,
			"PreMaster":   master,
			"PreService":  service,
			"Title":       "Запись — СЕВЕР",
			"Today":       time.Now().Format("2006-01-02"),
			"MinDate":     time.Now().Format("2006-01-02"),
			"MaxDate":     time.Now().AddDate(0, 0, 21).Format("2006-01-02"),
			"TelegramURL": telegramBotURL(),
		}
		if err := tmpl.ExecuteTemplate(w, "book.html", data); err != nil {
			log.Println(err)
			http.Error(w, "template error", 500)
		}
	})

	mux.HandleFunc("/my", func(w http.ResponseWriter, r *http.Request) {
		phone := r.URL.Query().Get("phone")
		var list []Booking
		if phone != "" {
			list = app.ByPhone(phone)
		}
		data := map[string]any{
			"Phone":    phone,
			"Bookings": list,
			"Title":    "Мои записи — СЕВЕР",
		}
		if err := tmpl.ExecuteTemplate(w, "my.html", data); err != nil {
			log.Println(err)
			http.Error(w, "template error", 500)
		}
	})

	mux.HandleFunc("/api/slots", func(w http.ResponseWriter, r *http.Request) {
		master := r.URL.Query().Get("master")
		date := r.URL.Query().Get("date")
		if master == "" || date == "" {
			http.Error(w, "master and date required", 400)
			return
		}
		slots := generateSlots(date)
		var free []string
		for _, t := range slots {
			if !app.IsTaken(master, date, t) {
				free = append(free, t)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"slots": free})
	})

	mux.HandleFunc("/api/book", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		handleBookPost(w, r, app)
	})

	mux.HandleFunc("/telegram/webhook", handleTelegramWebhook(app))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	log.Printf("СЕВЕР barbershop listening on %s", addr)
	if telegramEnabled() {
		log.Println("Telegram notifications: enabled")
	}
	setupTelegramWebhook()
	log.Fatal(http.ListenAndServe(addr, mux))
}

func generateSlots(date string) []string {
	var slots []string
	start := 10 * 60
	end := 20 * 60
	for m := start; m < end; m += 30 {
		if m >= 14*60 && m < 15*60 {
			continue
		}
		h := m / 60
		mm := m % 60
		slots = append(slots, fmt.Sprintf("%02d:%02d", h, mm))
	}
	today := time.Now().Format("2006-01-02")
	if date == today {
		now := time.Now()
		cutoff := now.Hour()*60 + now.Minute() + 30
		var filtered []string
		for _, s := range slots {
			parts := strings.Split(s, ":")
			hh, _ := strconv.Atoi(parts[0])
			mm, _ := strconv.Atoi(parts[1])
			if hh*60+mm >= cutoff {
				filtered = append(filtered, s)
			}
		}
		return filtered
	}
	return slots
}

func telegramEnabled() bool {
	return os.Getenv("TELEGRAM_BOT_TOKEN") != "" && os.Getenv("TELEGRAM_CHAT_ID") != ""
}

func formatRuDate(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	months := []string{"", "января", "февраля", "марта", "апреля", "мая", "июня",
		"июля", "августа", "сентября", "октября", "ноября", "декабря"}
	weekdays := []string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}
	return fmt.Sprintf("%d %s (%s)", t.Day(), months[int(t.Month())], weekdays[t.Weekday()])
}

func notifyTelegram(b Booking) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if token == "" || chatID == "" {
		return
	}

	master := findMasterName(b.MasterID)
	service := findServiceName(b.ServiceID)
	dateRu := formatRuDate(b.Date)

	text := fmt.Sprintf(
		"┌─────────────────────\n"+
			"│  <b>СЕВЕР</b> · новая запись\n"+
			"└─────────────────────\n\n"+
			"<b>%s</b>\n"+
			"%s · %s\n\n"+
			"━━━━━━━━━━━━━━━━━━━━\n"+
			"👤  <b>%s</b>\n"+
			"📞  <code>%s</code>\n"+
			"✂️  %s\n"+
			"🧔  мастер %s\n"+
			"━━━━━━━━━━━━━━━━━━━━\n\n"+
			"<i>id</i> <code>%s</code>",
		dateRu,
		b.Time,
		htmlEscape(service),
		htmlEscape(b.Name),
		htmlEscape(b.Phone),
		htmlEscape(service),
		htmlEscape(master),
		b.ID,
	)

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("parse_mode", "HTML")

	resp, err := http.PostForm(apiURL, form)
	if err != nil {
		log.Println("telegram notify:", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		log.Println("telegram error:", resp.StatusCode, buf.String())
	}
}

func findMasterName(id string) string {
	for _, m := range masters {
		if m.ID == id {
			return m.Name
		}
	}
	return id
}

func findServiceName(id string) string {
	for _, s := range services {
		if s.ID == id {
			return s.Name
		}
	}
	return id
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func telegramBotURL() string {
	u := os.Getenv("TELEGRAM_BOT_USERNAME")
	if u == "" {
		return ""
	}
	u = strings.TrimPrefix(u, "@")
	return "https://t.me/" + u
}

func handleBookPost(w http.ResponseWriter, r *http.Request, app *App) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	b := Booking{
		MasterID:  r.FormValue("master"),
		ServiceID: r.FormValue("service"),
		Date:      r.FormValue("date"),
		Time:      r.FormValue("time"),
		Name:      strings.TrimSpace(r.FormValue("name")),
		Phone:     strings.TrimSpace(r.FormValue("phone")),
	}
	if b.MasterID == "" || b.ServiceID == "" || b.Date == "" || b.Time == "" || b.Name == "" || b.Phone == "" {
		http.Error(w, "заполните все поля", 400)
		return
	}
	if err := app.Add(b); err != nil {
		http.Error(w, "это время уже занято, выберите другое", 409)
		return
	}

	go notifyTelegram(b)

	http.Redirect(w, r, "/my?phone="+url.QueryEscape(b.Phone)+"&ok=1", http.StatusSeeOther)
}
