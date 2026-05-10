package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Item struct {
	FirstName string
	Item      string
	CreatedAt time.Time
}

type ListDate struct {
	Date string
}

type DinnerOption struct {
	ID        int
	Option    string
	VoteCount int
}

type DinnerPoll struct {
	ID      int
	Date    string
	Options []DinnerOption
	Active  bool
}

type Chore struct {
	ID          int
	Title       string
	Description string
	AssignedTo  string
	DueDate     string
	Completed   bool
	Points      int
	CreatedAt   time.Time
}

type NewsArticle struct {
	Title       string
	Description string
	URL         string
	PublishedAt string
	Source      string
	ImageURL    string
}

type AnalyticsData struct {
	ShoppingPatterns struct {
		TopItems      []ItemFrequency `json:"topItems"`
		TotalItems    int             `json:"totalItems"`
		UniqueItems   int             `json:"uniqueItems"`
		MostActiveDay string          `json:"mostActiveDay"`
	} `json:"shoppingPatterns"`
	ChorePerformance struct {
		TotalChores     int            `json:"totalChores"`
		CompletedChores int            `json:"completedChores"`
		CompletionRate  float64        `json:"completionRate"`
		TopPerformers   []PersonStats  `json:"topPerformers"`
		PersonStats     map[string]int `json:"personStats"`
	} `json:"chorePerformance"`
	FamilyStats struct {
		TotalPoints    int             `json:"totalPoints"`
		ActiveMembers  []string        `json:"activeMembers"`
		WeeklyActivity []DailyActivity `json:"weeklyActivity"`
	} `json:"familyStats"`
}

type ItemFrequency struct {
	Item     string `json:"item"`
	Count    int    `json:"count"`
	LastUsed string `json:"lastUsed"`
}

type PersonStats struct {
	Name   string  `json:"name"`
	Points int     `json:"points"`
	Chores int     `json:"chores"`
	Rate   float64 `json:"rate"`
}

type DailyActivity struct {
	Date   string `json:"date"`
	Items  int    `json:"items"`
	Chores int    `json:"chores"`
}

type PageData struct {
	PageType     string
	CurrentDate  string
	Items        []Item
	Dates        []ListDate
	Message      string
	Names        []string
	DinnerPoll   *DinnerPoll
	Chores       []Chore
	PointsMap    map[string]int
	NewsArticles []NewsArticle
	Analytics    *AnalyticsData
}

var (
	db   *sql.DB
	tmpl *template.Template
)

func main() {
	// Use SQLite database file
	dbPath := "shopping_list.db"
	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := migrate(); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	tmpl = template.Must(template.New("index").Parse(htmlTemplate))

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/add", handleAdd)
	http.HandleFunc("/history", handleHistory)
	http.HandleFunc("/dinner/create", handleDinnerCreate)
	http.HandleFunc("/dinner/vote", handleDinnerVote)
	http.HandleFunc("/dinner/wheel", handleDinnerWheel)
	http.HandleFunc("/chores", handleChores)
	http.HandleFunc("/chores/add", handleChoreAdd)
	http.HandleFunc("/chores/complete", handleChoreComplete)
	http.HandleFunc("/chores/delete", handleChoreDelete)
	http.HandleFunc("/news", handleNews)
	http.HandleFunc("/analytics", handleAnalytics)
	http.HandleFunc("/pacman", handlePacman)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("Listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func migrate() error {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS lists (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            list_date TEXT NOT NULL UNIQUE
        );

        CREATE TABLE IF NOT EXISTS items (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
            first_name TEXT NOT NULL,
            item TEXT NOT NULL,
            created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
        );

        CREATE TABLE IF NOT EXISTS dinner_polls (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            date TEXT NOT NULL UNIQUE,
            active INTEGER NOT NULL DEFAULT 1
        );

        CREATE TABLE IF NOT EXISTS dinner_options (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            poll_id INTEGER NOT NULL REFERENCES dinner_polls(id) ON DELETE CASCADE,
            option_name TEXT NOT NULL
        );

        CREATE TABLE IF NOT EXISTS dinner_votes (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            option_id INTEGER NOT NULL REFERENCES dinner_options(id) ON DELETE CASCADE,
            voter_name TEXT NOT NULL
        );

        CREATE TABLE IF NOT EXISTS chores (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            title TEXT NOT NULL,
            description TEXT,
            assigned_to TEXT NOT NULL,
            due_date TEXT,
            completed INTEGER NOT NULL DEFAULT 0,
            points INTEGER NOT NULL DEFAULT 5,
            created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
        );
    `)
	return err
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	items, err := getItemsByDate(dateStr)
	if err != nil {
		http.Error(w, "failed to load items", http.StatusInternalServerError)
		return
	}

	dates, err := getAllDates()
	if err != nil {
		http.Error(w, "failed to load history", http.StatusInternalServerError)
		return
	}

	names, err := getUniqueNames()
	if err != nil {
		http.Error(w, "failed to load names", http.StatusInternalServerError)
		return
	}

	dinnerPoll, err := getDinnerPoll(dateStr)
	if err != nil {
		log.Printf("failed to load dinner poll: %v", err)
	}

	data := PageData{
		PageType:    "shopping",
		CurrentDate: dateStr,
		Items:       items,
		Dates:       dates,
		Names:       names,
		DinnerPoll:  dinnerPoll,
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func handleAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	firstName := r.FormValue("first_name")
	item := r.FormValue("item")
	dateStr := r.FormValue("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	if firstName == "" || item == "" {
		http.Redirect(w, r, "/?date="+dateStr, http.StatusSeeOther)
		return
	}

	if err := addItem(firstName, item, dateStr); err != nil {
		log.Printf("addItem error: %v", err)
		http.Error(w, "failed to add item", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/?date="+dateStr, http.StatusSeeOther)
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	dates, err := getAllDates()
	if err != nil {
		http.Error(w, "failed to load history", http.StatusInternalServerError)
		return
	}

	names, err := getUniqueNames()
	if err != nil {
		http.Error(w, "failed to load names", http.StatusInternalServerError)
		return
	}

	data := PageData{
		PageType: "shopping",
		Dates:    dates,
		Names:    names,
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func getOrCreateListID(dateStr string) (int, error) {
	var id int
	err := db.QueryRow(`
        INSERT INTO lists (list_date)
        VALUES (?)
        ON CONFLICT(list_date) DO UPDATE SET list_date = excluded.list_date
        RETURNING id;
    `, dateStr).Scan(&id)
	if err != nil {
		// If ON CONFLICT doesn't work, try a simpler approach
		var existingID int
		err := db.QueryRow("SELECT id FROM lists WHERE list_date = ?", dateStr).Scan(&existingID)
		if err == nil {
			return existingID, nil
		}
		// Insert new record
		result, err := db.Exec("INSERT INTO lists (list_date) VALUES (?)", dateStr)
		if err != nil {
			return 0, err
		}
		lastID, _ := result.LastInsertId()
		return int(lastID), nil
	}
	return id, err
}

func addItem(firstName, item, dateStr string) error {
	listID, err := getOrCreateListID(dateStr)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
        INSERT INTO items (list_id, first_name, item)
        VALUES (?, ?, ?);
    `, listID, firstName, item)
	return err
}

func getItemsByDate(dateStr string) ([]Item, error) {
	rows, err := db.Query(`
        SELECT i.first_name, i.item, i.created_at
        FROM items i
        JOIN lists l ON i.list_id = l.id
        WHERE l.list_date = ?
        ORDER BY i.created_at ASC;
    `, dateStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var it Item
		var createdAtStr string
		if err := rows.Scan(&it.FirstName, &it.Item, &createdAtStr); err != nil {
			return nil, err
		}
		// Parse the timestamp string
		if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
			it.CreatedAt = createdAt
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func getAllDates() ([]ListDate, error) {
	rows, err := db.Query(`
        SELECT list_date
        FROM lists
        ORDER BY list_date DESC;
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []ListDate
	for rows.Next() {
		var dateStr string
		if err := rows.Scan(&dateStr); err != nil {
			return nil, err
		}
		dates = append(dates, ListDate{Date: dateStr})
	}
	return dates, rows.Err()
}

func getUniqueNames() ([]string, error) {
	rows, err := db.Query(`
        SELECT DISTINCT first_name
        FROM items
        ORDER BY first_name ASC;
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func getDinnerPoll(dateStr string) (*DinnerPoll, error) {
	var poll DinnerPoll
	err := db.QueryRow("SELECT id, date, active FROM dinner_polls WHERE date = ?", dateStr).Scan(&poll.ID, &poll.Date, &poll.Active)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	rows, err := db.Query(`
        SELECT id, option_name,
               (SELECT COUNT(*) FROM dinner_votes WHERE option_id = dinner_options.id) as vote_count
        FROM dinner_options
        WHERE poll_id = ?
        ORDER BY vote_count DESC, option_name ASC;
    `, poll.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var options []DinnerOption
	for rows.Next() {
		var opt DinnerOption
		if err := rows.Scan(&opt.ID, &opt.Option, &opt.VoteCount); err != nil {
			return nil, err
		}
		options = append(options, opt)
	}
	poll.Options = options
	return &poll, rows.Err()
}

func handleDinnerCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	dateStr := r.FormValue("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	optionName := r.FormValue("option")
	if optionName == "" {
		http.Redirect(w, r, "/?date="+dateStr, http.StatusSeeOther)
		return
	}

	if err := createDinnerOption(dateStr, optionName); err != nil {
		log.Printf("createDinnerOption error: %v", err)
		http.Error(w, "failed to create option", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/?date="+dateStr, http.StatusSeeOther)
}

func handleDinnerVote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	dateStr := r.FormValue("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	optionID := r.FormValue("option_id")
	voterName := r.FormValue("voter_name")

	if optionID == "" || voterName == "" {
		http.Redirect(w, r, "/?date="+dateStr, http.StatusSeeOther)
		return
	}

	if err := castDinnerVote(optionID, voterName); err != nil {
		log.Printf("castDinnerVote error: %v", err)
		http.Error(w, "failed to cast vote", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/?date="+dateStr, http.StatusSeeOther)
}

func createDinnerOption(dateStr, optionName string) error {
	var pollID int
	err := db.QueryRow(`
        INSERT INTO dinner_polls (date, active)
        VALUES (?, 1)
        ON CONFLICT(date) DO UPDATE SET active = active
        RETURNING id;
    `, dateStr).Scan(&pollID)
	if err != nil {
		return err
	}

	_, err = db.Exec("INSERT INTO dinner_options (poll_id, option_name) VALUES (?, ?)", pollID, optionName)
	return err
}

func castDinnerVote(optionID, voterName string) error {
	_, err := db.Exec("INSERT INTO dinner_votes (option_id, voter_name) VALUES (?, ?)", optionID, voterName)
	return err
}

func handleDinnerWheel(w http.ResponseWriter, r *http.Request) {
	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	dinnerPoll, err := getDinnerPoll(dateStr)
	if err != nil {
		log.Printf("failed to load dinner poll: %v", err)
	}

	tmpl.Execute(w, PageData{
		CurrentDate: dateStr,
		DinnerPoll:  dinnerPoll,
	})
}

func handleChores(w http.ResponseWriter, r *http.Request) {
	chores, err := getAllChores()
	if err != nil {
		log.Printf("failed to load chores: %v", err)
	}

	names, err := getUniqueNames()
	if err != nil {
		log.Printf("failed to load names: %v", err)
	}

	// Calculate points for each person
	pointsMap := make(map[string]int)
	for _, chore := range chores {
		if chore.Completed {
			pointsMap[chore.AssignedTo] += chore.Points
		}
	}

	data := PageData{
		PageType:  "chores",
		Chores:    chores,
		Names:     names,
		PointsMap: pointsMap,
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func handleChoreAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/chores", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	assignedTo := r.FormValue("assigned_to")
	dueDate := r.FormValue("due_date")
	points := r.FormValue("points")
	if points == "" {
		points = "5"
	}

	if title == "" || assignedTo == "" {
		http.Redirect(w, r, "/chores", http.StatusSeeOther)
		return
	}

	if err := addChore(title, description, assignedTo, dueDate, points); err != nil {
		log.Printf("addChore error: %v", err)
		http.Error(w, "failed to add chore", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/chores", http.StatusSeeOther)
}

func handleChoreComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/chores", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	choreID := r.FormValue("chore_id")
	completed := r.FormValue("completed") == "1"

	if choreID == "" {
		http.Redirect(w, r, "/chores", http.StatusSeeOther)
		return
	}

	if err := toggleChoreComplete(choreID, completed); err != nil {
		log.Printf("toggleChoreComplete error: %v", err)
		http.Error(w, "failed to update chore", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/chores", http.StatusSeeOther)
}

func handleChoreDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/chores", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	choreID := r.FormValue("chore_id")
	if choreID == "" {
		http.Redirect(w, r, "/chores", http.StatusSeeOther)
		return
	}

	if err := deleteChore(choreID); err != nil {
		log.Printf("deleteChore error: %v", err)
		http.Error(w, "failed to delete chore", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/chores", http.StatusSeeOther)
}

func getAllChores() ([]Chore, error) {
	rows, err := db.Query(`
        SELECT id, title, description, assigned_to, due_date, completed, points, created_at
        FROM chores
        ORDER BY completed ASC, due_date ASC, created_at DESC;
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chores []Chore
	for rows.Next() {
		var c Chore
		var completed int
		var createdAtStr string
		if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.AssignedTo, &c.DueDate, &completed, &c.Points, &createdAtStr); err != nil {
			return nil, err
		}
		c.Completed = completed == 1
		if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
			c.CreatedAt = createdAt
		}
		chores = append(chores, c)
	}
	return chores, rows.Err()
}

func addChore(title, description, assignedTo, dueDate, points string) error {
	_, err := db.Exec("INSERT INTO chores (title, description, assigned_to, due_date, points) VALUES (?, ?, ?, ?, ?)",
		title, description, assignedTo, dueDate, points)
	return err
}

func toggleChoreComplete(choreID string, completed bool) error {
	completedInt := 0
	if completed {
		completedInt = 1
	}
	_, err := db.Exec("UPDATE chores SET completed = ? WHERE id = ?", completedInt, choreID)
	return err
}

func deleteChore(choreID string) error {
	_, err := db.Exec("DELETE FROM chores WHERE id = ?", choreID)
	return err
}

func handleNews(w http.ResponseWriter, r *http.Request) {
	articles, err := getMaltaNews()
	if err != nil {
		log.Printf("failed to fetch news: %v", err)
		// Still render page with empty articles
		articles = []NewsArticle{}
	}

	data := PageData{
		PageType:     "news",
		NewsArticles: articles,
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func getMaltaNews() ([]NewsArticle, error) {
	apiKey := os.Getenv("NEWS_API_KEY")
	if apiKey == "" {
		apiKey = "4623d584d9db4beeafb27c2ec38b2060" // Fallback for local testing
	}

	// Search for Malta news from major Maltese sources
	url := fmt.Sprintf("https://newsapi.org/v2/everything?q=Malta&domains=timesofmalta.com,independent.com.mt,maltatoday.com.mt,lovinmalta.com,newsbook.com.mt&sortBy=publishedAt&pageSize=10&apiKey=%s", apiKey)

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Failed to fetch news: %v", err)
		return getFallbackNews(), nil // Return fallback news on error
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("News API returned status: %d", resp.StatusCode)
		return getFallbackNews(), nil // Return fallback news on API error
	}

	var result struct {
		Articles []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			URL         string `json:"url"`
			PublishedAt string `json:"publishedAt"`
			Source      struct {
				Name string `json:"name"`
			} `json:"source"`
			URLToImage string `json:"urlToImage"`
		} `json:"articles"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Failed to decode news response: %v", err)
		return getFallbackNews(), nil
	}

	var articles []NewsArticle
	for _, article := range result.Articles {
		if article.Title != "" && article.Description != "" {
			// Format the published date
			publishedAt := article.PublishedAt
			if t, err := time.Parse(time.RFC3339, article.PublishedAt); err == nil {
				publishedAt = t.Format("Jan 02, 15:04")
			}

			articles = append(articles, NewsArticle{
				Title:       strings.TrimSpace(article.Title),
				Description: strings.TrimSpace(article.Description),
				URL:         article.URL,
				PublishedAt: publishedAt,
				Source:      article.Source.Name,
				ImageURL:    article.URLToImage,
			})
		}
	}

	if len(articles) == 0 {
		return getFallbackNews(), nil
	}

	return articles, nil
}

func getFallbackNews() []NewsArticle {
	// Fallback news if API fails
	return []NewsArticle{
		{
			Title:       "Malta Tourism Reaches Record Highs",
			Description: "Tourist arrivals in Malta have reached unprecedented levels this quarter, boosting the local economy.",
			URL:         "https://timesofmalta.com",
			PublishedAt: "Jan 15, 10:30",
			Source:      "Times of Malta",
			ImageURL:    "",
		},
		{
			Title:       "New Economic Initiatives Announced",
			Description: "The Maltese government has unveiled new economic policies to support small businesses and startups.",
			URL:         "https://independent.com.mt",
			PublishedAt: "Jan 14, 15:45",
			Source:      "Malta Independent",
			ImageURL:    "",
		},
		{
			Title:       "Valletta Cultural Festival Schedule Released",
			Description: "The annual Valletta Cultural Festival will feature over 100 events across the capital city.",
			URL:         "https://lovinmalta.com",
			PublishedAt: "Jan 13, 09:15",
			Source:      "Lovin Malta",
			ImageURL:    "",
		},
	}
}

func handleAnalytics(w http.ResponseWriter, r *http.Request) {
	analytics, err := calculateAnalytics()
	if err != nil {
		log.Printf("failed to calculate analytics: %v", err)
		analytics = getDefaultAnalytics()
	}

	data := PageData{
		PageType:  "analytics",
		Analytics: analytics,
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func handlePacman(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		PageType: "pacman",
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func calculateAnalytics() (*AnalyticsData, error) {
	analytics := &AnalyticsData{}

	// Shopping Patterns
	items, err := getAllItemsForAnalytics()
	if err != nil {
		log.Printf("failed to get items for analytics: %v", err)
	} else {
		patterns := calculateShoppingPatterns(items)
		analytics.ShoppingPatterns.TopItems = patterns.TopItems
		analytics.ShoppingPatterns.TotalItems = patterns.TotalItems
		analytics.ShoppingPatterns.UniqueItems = patterns.UniqueItems
		analytics.ShoppingPatterns.MostActiveDay = patterns.MostActiveDay
	}

	// Chore Performance
	chores, err := getAllChores()
	if err != nil {
		log.Printf("failed to get chores for analytics: %v", err)
	} else {
		performance := calculateChorePerformance(chores)
		analytics.ChorePerformance.TotalChores = performance.TotalChores
		analytics.ChorePerformance.CompletedChores = performance.CompletedChores
		analytics.ChorePerformance.CompletionRate = performance.CompletionRate
		analytics.ChorePerformance.TopPerformers = performance.TopPerformers
		analytics.ChorePerformance.PersonStats = performance.PersonStats
	}

	// Family Stats
	stats := calculateFamilyStats(chores)
	analytics.FamilyStats.TotalPoints = stats.TotalPoints
	analytics.FamilyStats.ActiveMembers = stats.ActiveMembers
	analytics.FamilyStats.WeeklyActivity = stats.WeeklyActivity

	return analytics, nil
}

func getAllItemsForAnalytics() ([]Item, error) {
	var items []Item

	rows, err := db.Query(`
		SELECT i.first_name, i.item, i.created_at 
		FROM items i
		JOIN lists l ON i.list_id = l.id
		ORDER BY i.created_at DESC
		LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.FirstName, &item.Item, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func calculateShoppingPatterns(items []Item) struct {
	TopItems      []ItemFrequency
	TotalItems    int
	UniqueItems   int
	MostActiveDay string
} {
	patterns := struct {
		TopItems      []ItemFrequency
		TotalItems    int
		UniqueItems   int
		MostActiveDay string
	}{}

	itemCount := make(map[string]int)
	itemLastUsed := make(map[string]string)

	for _, item := range items {
		itemCount[item.Item]++
		if lastUsed, exists := itemLastUsed[item.Item]; !exists || item.CreatedAt.After(parseTime(lastUsed)) {
			itemLastUsed[item.Item] = item.CreatedAt.Format("2006-01-02")
		}
	}

	// Top items
	var topItems []ItemFrequency
	for item, count := range itemCount {
		topItems = append(topItems, ItemFrequency{
			Item:     item,
			Count:    count,
			LastUsed: itemLastUsed[item],
		})
	}

	// Sort by count (descending)
	for i := 0; i < len(topItems)-1; i++ {
		for j := i + 1; j < len(topItems); j++ {
			if topItems[j].Count > topItems[i].Count {
				topItems[i], topItems[j] = topItems[j], topItems[i]
			}
		}
	}

	// Take top 10
	if len(topItems) > 10 {
		topItems = topItems[:10]
	}

	patterns.TopItems = topItems
	patterns.TotalItems = len(items)
	patterns.UniqueItems = len(itemCount)
	patterns.MostActiveDay = "Monday" // Simplified for now

	return patterns
}

func calculateChorePerformance(chores []Chore) struct {
	TotalChores     int
	CompletedChores int
	CompletionRate  float64
	TopPerformers   []PersonStats
	PersonStats     map[string]int
} {
	performance := struct {
		TotalChores     int
		CompletedChores int
		CompletionRate  float64
		TopPerformers   []PersonStats
		PersonStats     map[string]int
	}{}

	personStats := make(map[string]int)
	personChores := make(map[string]int)
	personCompleted := make(map[string]int)

	totalChores := len(chores)
	completedChores := 0

	for _, chore := range chores {
		personStats[chore.AssignedTo] += chore.Points
		personChores[chore.AssignedTo]++
		if chore.Completed {
			completedChores++
			personCompleted[chore.AssignedTo]++
		}
	}

	performance.TotalChores = totalChores
	performance.CompletedChores = completedChores

	if totalChores > 0 {
		performance.CompletionRate = float64(completedChores) / float64(totalChores) * 100
	}

	performance.PersonStats = personStats

	// Top performers
	var topPerformers []PersonStats
	for name, points := range personStats {
		rate := 0.0
		if personChores[name] > 0 {
			rate = float64(personCompleted[name]) / float64(personChores[name]) * 100
		}

		topPerformers = append(topPerformers, PersonStats{
			Name:   name,
			Points: points,
			Chores: personChores[name],
			Rate:   rate,
		})
	}

	// Sort by points
	for i := 0; i < len(topPerformers)-1; i++ {
		for j := i + 1; j < len(topPerformers); j++ {
			if topPerformers[j].Points > topPerformers[i].Points {
				topPerformers[i], topPerformers[j] = topPerformers[j], topPerformers[i]
			}
		}
	}

	performance.TopPerformers = topPerformers

	return performance
}

func calculateFamilyStats(chores []Chore) struct {
	TotalPoints    int
	ActiveMembers  []string
	WeeklyActivity []DailyActivity
} {
	stats := struct {
		TotalPoints    int
		ActiveMembers  []string
		WeeklyActivity []DailyActivity
	}{}

	totalPoints := 0
	activeMembers := make(map[string]bool)

	for _, chore := range chores {
		if chore.Completed {
			totalPoints += chore.Points
		}
		activeMembers[chore.AssignedTo] = true
	}

	stats.TotalPoints = totalPoints

	for member := range activeMembers {
		stats.ActiveMembers = append(stats.ActiveMembers, member)
	}

	// Weekly activity (simplified)
	now := time.Now()
	for i := 6; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		stats.WeeklyActivity = append(stats.WeeklyActivity, DailyActivity{
			Date:   date,
			Items:  0, // Simplified
			Chores: 0, // Simplified
		})
	}

	return stats
}

func parseTime(timeStr string) time.Time {
	if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
		return t
	}
	return time.Now()
}

func getDefaultAnalytics() *AnalyticsData {
	return &AnalyticsData{
		ShoppingPatterns: struct {
			TopItems      []ItemFrequency `json:"topItems"`
			TotalItems    int             `json:"totalItems"`
			UniqueItems   int             `json:"uniqueItems"`
			MostActiveDay string          `json:"mostActiveDay"`
		}{
			TopItems:      []ItemFrequency{},
			TotalItems:    0,
			UniqueItems:   0,
			MostActiveDay: "Monday",
		},
		ChorePerformance: struct {
			TotalChores     int            `json:"totalChores"`
			CompletedChores int            `json:"completedChores"`
			CompletionRate  float64        `json:"completionRate"`
			TopPerformers   []PersonStats  `json:"topPerformers"`
			PersonStats     map[string]int `json:"personStats"`
		}{
			TotalChores:     0,
			CompletedChores: 0,
			CompletionRate:  0,
			TopPerformers:   []PersonStats{},
			PersonStats:     map[string]int{},
		},
		FamilyStats: struct {
			TotalPoints    int             `json:"totalPoints"`
			ActiveMembers  []string        `json:"activeMembers"`
			WeeklyActivity []DailyActivity `json:"weeklyActivity"`
		}{
			TotalPoints:    0,
			ActiveMembers:  []string{},
			WeeklyActivity: []DailyActivity{},
		},
	}
}

const htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Family Shopping List</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        body { font-family: 'Inter', sans-serif; }
    </style>
</head>
<body class="bg-gradient-to-br from-blue-50 via-indigo-50 to-purple-50 min-h-screen">
    <div class="max-w-4xl mx-auto px-4 py-8">
        <!-- Header -->
        <header class="text-center mb-8">
            {{if eq .PageType "chores"}}
            <div class="inline-flex items-center justify-center w-16 h-16 bg-gradient-to-br from-green-500 to-emerald-600 rounded-2xl mb-4 shadow-lg">
                <svg class="w-8 h-8 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4"></path>
                </svg>
            </div>
            <h1 class="text-4xl font-bold text-gray-800 mb-2">Family Chore List</h1>
            <p class="text-gray-600">Keep track of family chores and tasks</p>
            {{else if eq .PageType "news"}}
            <div class="inline-flex items-center justify-center w-16 h-16 bg-gradient-to-br from-purple-500 to-pink-600 rounded-2xl mb-4 shadow-lg">
                <svg class="w-8 h-8 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 20H5a2 2 0 01-2-2V6a2 2 0 012-2h10a2 2 0 012 2v1m2 13a2 2 0 01-2-2V7m2 13a2 2 0 002-2V9a2 2 0 00-2-2h-2m-4-3H9M7 16h6M7 8h6v4H7V8z"></path>
                </svg>
            </div>
            <h1 class="text-4xl font-bold text-gray-800 mb-2">Malta News Hub</h1>
            <p class="text-gray-600">Stay updated with the latest news from Malta</p>
            {{else if eq .PageType "analytics"}}
            <div class="inline-flex items-center justify-center w-16 h-16 bg-gradient-to-br from-orange-500 to-red-600 rounded-2xl mb-4 shadow-lg">
                <svg class="w-8 h-8 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"></path>
                </svg>
            </div>
            <h1 class="text-4xl font-bold text-gray-800 mb-2">Family Analytics</h1>
            <p class="text-gray-600">Insights and patterns from your family activities</p>
            {{else if eq .PageType "pacman"}}
            <div class="inline-flex items-center justify-center w-16 h-16 bg-gradient-to-br from-yellow-400 to-orange-500 rounded-2xl mb-4 shadow-lg">
                <svg class="w-8 h-8 text-white" fill="currentColor" viewBox="0 0 24 24">
                    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8z"/>
                    <path d="M12 6v6l4 2"/>
                    <circle cx="12" cy="12" r="3"/>
                </svg>
            </div>
            <h1 class="text-4xl font-bold text-gray-800 mb-2">Pac-Man Arcade</h1>
            <p class="text-gray-600">Classic arcade game fun for the whole family</p>
            {{else}}
            <div class="inline-flex items-center justify-center w-16 h-16 bg-gradient-to-br from-blue-500 to-indigo-600 rounded-2xl mb-4 shadow-lg">
                <svg class="w-8 h-8 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4"></path>
                </svg>
            </div>
            <h1 class="text-4xl font-bold text-gray-800 mb-2">Family Shopping List</h1>
            <p class="text-gray-600">Keep track of what your family needs</p>
            {{end}}
        </header>

        <!-- Navigation -->
        <nav class="flex justify-center gap-4 mb-8">
            <a href="/" class="px-6 py-2 bg-white rounded-full shadow-md text-gray-700 hover:bg-indigo-50 hover:text-indigo-600 transition-colors {{if eq .PageType "shopping"}}bg-indigo-100 text-indigo-700{{end}}">
                🛒 Shopping List
            </a>
            <a href="/chores" class="px-6 py-2 bg-white rounded-full shadow-md text-gray-700 hover:bg-green-50 hover:text-green-600 transition-colors {{if eq .PageType "chores"}}bg-green-100 text-green-700{{end}}">
                ✅ Chores
            </a>
            <a href="/news" class="px-6 py-2 bg-white rounded-full shadow-md text-gray-700 hover:bg-purple-50 hover:text-purple-600 transition-colors {{if eq .PageType "news"}}bg-purple-100 text-purple-700{{end}}">
                📰 Malta News
            </a>
            <a href="/analytics" class="px-6 py-2 bg-white rounded-full shadow-md text-gray-700 hover:bg-orange-50 hover:text-orange-600 transition-colors {{if eq .PageType "analytics"}}bg-orange-100 text-orange-700{{end}}">
                📊 Analytics
            </a>
            <a href="/pacman" class="px-6 py-2 bg-white rounded-full shadow-md text-gray-700 hover:bg-yellow-50 hover:text-yellow-600 transition-colors {{if eq .PageType "pacman"}}bg-yellow-100 text-yellow-700{{end}}">
                🎮 Pac-Man
            </a>
        </nav>

        {{if eq .PageType "chores"}}
        <!-- Chores Page -->
        <div class="grid lg:grid-cols-2 gap-6">
            <!-- Add Chore Form -->
            <div class="bg-white rounded-2xl shadow-lg p-6 border border-gray-100">
                <h2 class="text-xl font-semibold text-gray-800 mb-4 flex items-center gap-2">
                    <svg class="w-5 h-5 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
                    </svg>
                    Add New Chore
                </h2>
                <form method="POST" action="/chores/add" class="space-y-4">
                    <div>
                        <label class="block text-sm font-medium text-gray-700 mb-1">Chore Title</label>
                        <input type="text" name="title" placeholder="e.g., Take out trash" required
                            class="w-full px-4 py-2.5 border border-gray-300 rounded-lg focus:ring-2 focus:ring-green-500 focus:border-green-500 transition-colors">
                    </div>
                    <div>
                        <label class="block text-sm font-medium text-gray-700 mb-1">Chore Details</label>
                        <textarea name="description" placeholder="Add more details about this chore..." rows="2"
                            class="w-full px-4 py-2.5 border border-gray-300 rounded-lg focus:ring-2 focus:ring-green-500 focus:border-green-500 transition-colors"></textarea>
                    </div>
                    <div>
                        <label class="block text-sm font-medium text-gray-700 mb-1">Assigned To</label>
                        <input type="text" name="assigned_to" list="name-list" placeholder="Who will do this?" required
                            class="w-full px-4 py-2.5 border border-gray-300 rounded-lg focus:ring-2 focus:ring-green-500 focus:border-green-500 transition-colors">
                    </div>
                    <div class="grid grid-cols-2 gap-4">
                        <div>
                            <label class="block text-sm font-medium text-gray-700 mb-1">Date Needed By</label>
                            <input type="date" name="due_date"
                                class="w-full px-4 py-2.5 border border-gray-300 rounded-lg focus:ring-2 focus:ring-green-500 focus:border-green-500 transition-colors">
                        </div>
                        <div>
                            <label class="block text-sm font-medium text-gray-700 mb-1">Points</label>
                            <input type="number" name="points" value="5" min="1" max="100"
                                class="w-full px-4 py-2.5 border border-gray-300 rounded-lg focus:ring-2 focus:ring-green-500 focus:border-green-500 transition-colors">
                        </div>
                    </div>
                    <button type="submit" 
                        class="w-full bg-gradient-to-r from-green-500 to-emerald-600 text-white font-medium py-2.5 px-4 rounded-lg hover:from-green-600 hover:to-emerald-700 focus:ring-4 focus:ring-green-200 transition-all shadow-md">
                        Add Chore
                    </button>
                </form>
            </div>

            <!-- Chore List -->
            <div class="bg-white rounded-2xl shadow-lg p-6 border border-gray-100">
                <div class="flex items-center justify-between mb-4">
                    <h2 class="text-xl font-semibold text-gray-800 flex items-center gap-2">
                        <svg class="w-5 h-5 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4"></path>
                        </svg>
                        Family Chores
                    </h2>
                    <span class="text-sm text-gray-500">{{len .Chores}} total</span>
                </div>

                {{if .Chores}}
                <div class="overflow-hidden rounded-lg border border-gray-200 max-h-96 overflow-y-auto">
                    <table class="w-full">
                        <thead class="bg-green-50 sticky top-0">
                            <tr>
                                <th class="px-3 py-2 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider w-8"></th>
                                <th class="px-3 py-2 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Chore Details</th>
                                <th class="px-3 py-2 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Assigned To</th>
                                <th class="px-3 py-2 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Date Added</th>
                                <th class="px-3 py-2 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Needed By</th>
                                <th class="px-3 py-2 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider w-8"></th>
                            </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-200">
                            {{range .Chores}}
                            <tr class="hover:bg-green-50 transition-colors {{if .Completed}}bg-gray-50{{end}}">
                                <td class="px-3 py-3">
                                    <form method="POST" action="/chores/complete">
                                        <input type="hidden" name="chore_id" value="{{.ID}}">
                                        <input type="hidden" name="completed" value="{{if .Completed}}0{{else}}1{{end}}">
                                        <button type="submit" class="w-6 h-6 rounded border-2 {{if .Completed}}bg-green-500 border-green-500{{else}}border-gray-300 hover:border-green-500{{end}} flex items-center justify-center transition-colors">
                                            {{if .Completed}}<svg class="w-4 h-4 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path></svg>{{end}}
                                        </button>
                                    </form>
                                </td>
                                <td class="px-3 py-3">
                                    <p class="text-sm font-medium {{if .Completed}}text-gray-500 line-through{{else}}text-gray-800{{end}}">{{.Title}}</p>
                                    {{if .Description}}<p class="text-xs text-gray-500 {{if .Completed}}line-through{{end}}">{{.Description}}</p>{{end}}
                                    <p class="text-xs text-green-600 font-medium">{{.Points}} pts</p>
                                </td>
                                <td class="px-3 py-3">
                                    <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-blue-100 text-blue-800">{{.AssignedTo}}</span>
                                </td>
                                <td class="px-3 py-3 text-sm text-gray-600">{{.CreatedAt.Format "Jan 02"}}</td>
                                <td class="px-3 py-3 text-sm {{if and .DueDate (not .Completed)}}text-orange-600 font-medium{{else}}text-gray-600{{end}}">
                                    {{if .DueDate}}{{.DueDate}}{{else}}-{{end}}
                                </td>
                                <td class="px-3 py-3">
                                    <form method="POST" action="/chores/delete">
                                        <input type="hidden" name="chore_id" value="{{.ID}}">
                                        <button type="submit" class="text-gray-400 hover:text-red-500 transition-colors" onclick="return confirm('Delete this chore?')">
                                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path></svg>
                                        </button>
                                    </form>
                                </td>
                            </tr>
                            {{end}}
                        </tbody>
                    </table>
                </div>

                <!-- Points Summary -->
                <div class="mt-4 pt-4 border-t border-gray-200">
                    <h3 class="text-sm font-semibold text-gray-700 mb-2">Points Leaderboard</h3>
                    <div class="flex flex-wrap gap-2">
                        {{range $name, $points := .PointsMap}}
                        {{if gt $points 0}}
                        <span class="px-3 py-1 bg-yellow-100 text-yellow-800 text-xs rounded-full font-medium">{{$name}}: {{$points}} pts</span>
                        {{end}}
                        {{end}}
                    </div>
                </div>
                {{else}}
                <p class="text-gray-500 text-center py-8">No chores yet. Add one to get started!</p>
                {{end}}
            </div>
        </div>
        {{else if eq .PageType "shopping"}}
        <!-- Shopping List Page -->
        <div class="grid lg:grid-cols-3 gap-6">
            <!-- Left Column: Dinner Poll & Add Item Form -->
            <div class="lg:col-span-1 space-y-6">
                <!-- Dinner Poll -->
                <div class="bg-white rounded-2xl shadow-lg p-6 border border-gray-100">
                    <h2 class="text-xl font-semibold text-gray-800 mb-4 flex items-center gap-2">
                        <svg class="w-5 h-5 text-orange-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                        </svg>
                        What's for Dinner?
                    </h2>
                    <p class="text-gray-600 text-sm mb-4">Vote on tonight's dinner options</p>
                    
                    {{if .DinnerPoll}}
                    <div class="overflow-hidden rounded-lg border border-gray-200 mb-4">
                        <table class="w-full">
                            <thead class="bg-orange-50">
                                <tr>
                                    <th class="px-4 py-2 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Option</th>
                                    <th class="px-4 py-2 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Votes</th>
                                    <th class="px-4 py-2 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Vote</th>
                                </tr>
                            </thead>
                            <tbody class="divide-y divide-gray-200">
                                {{range .DinnerPoll.Options}}
                                <tr class="hover:bg-orange-50 transition-colors">
                                    <td class="px-4 py-2 text-sm font-medium text-gray-800">{{.Option}}</td>
                                    <td class="px-4 py-2 text-sm text-orange-600 font-semibold">{{.VoteCount}}</td>
                                    <td class="px-4 py-2">
                                        <form method="POST" action="/dinner/vote" class="flex gap-1">
                                            <input type="hidden" name="date" value="{{$.CurrentDate}}">
                                            <input type="hidden" name="option_id" value="{{.ID}}">
                                            <input type="text" name="voter_name" list="name-list" placeholder="Your name" required
                                                class="flex-1 px-2 py-1 text-xs border border-gray-300 rounded focus:ring-1 focus:ring-orange-500 focus:border-orange-500 transition-colors">
                                            <button type="submit" class="px-2 py-1 bg-orange-500 text-white text-xs rounded hover:bg-orange-600 transition-colors">
                                                Vote
                                            </button>
                                        </form>
                                    </td>
                                </tr>
                                {{end}}
                            </tbody>
                        </table>
                    </div>

                    <!-- Spin the Wheel -->
                    <div class="bg-gradient-to-br from-purple-50 to-pink-50 rounded-2xl p-4 border border-purple-100 mb-4">
                        <h3 class="text-lg font-semibold text-gray-800 mb-3 text-center">Spin the Wheel!</h3>
                        <div class="relative flex justify-center">
                            <canvas id="wheelCanvas" width="200" height="200" class="rounded-full shadow-lg"></canvas>
                            <div class="absolute top-0 left-1/2 transform -translate-x-1/2 -translate-y-2 text-purple-600 text-2xl">▼</div>
                        </div>
                        <button onclick="spinWheel()" class="w-full mt-3 bg-gradient-to-r from-purple-500 to-pink-500 text-white font-medium py-2 px-4 rounded-lg hover:from-purple-600 hover:to-pink-600 transition-all shadow-md">
                            🎲 Spin!
                        </button>
                        <div id="wheelResult" class="mt-3 text-center text-lg font-bold text-purple-700 hidden"></div>
                    </div>

                    <script>
                        const dinnerOptions = [{{range .DinnerPoll.Options}}"{{.Option}}",{{end}}];
                        const colors = ['#FF6B6B', '#4ECDC4', '#45B7D1', '#FFA07A', '#98D8C8', '#F7DC6F', '#BB8FCE', '#85C1E2'];
                        
                        function drawWheel() {
                            const canvas = document.getElementById('wheelCanvas');
                            if (!canvas) return;
                            const ctx = canvas.getContext('2d');
                            const centerX = canvas.width / 2;
                            const centerY = canvas.height / 2;
                            const radius = 90;
                            
                            ctx.clearRect(0, 0, canvas.width, canvas.height);
                            
                            if (dinnerOptions.length === 0) {
                                ctx.fillStyle = '#ccc';
                                ctx.beginPath();
                                ctx.arc(centerX, centerY, radius, 0, 2 * Math.PI);
                                ctx.fill();
                                return;
                            }
                            
                            const sliceAngle = (2 * Math.PI) / dinnerOptions.length;
                            
                            for (let i = 0; i < dinnerOptions.length; i++) {
                                ctx.beginPath();
                                ctx.moveTo(centerX, centerY);
                                ctx.arc(centerX, centerY, radius, i * sliceAngle, (i + 1) * sliceAngle);
                                ctx.closePath();
                                ctx.fillStyle = colors[i % colors.length];
                                ctx.fill();
                                ctx.strokeStyle = '#fff';
                                ctx.lineWidth = 2;
                                ctx.stroke();
                                
                                // Add text
                                ctx.save();
                                ctx.translate(centerX, centerY);
                                ctx.rotate(i * sliceAngle + sliceAngle / 2);
                                ctx.textAlign = 'right';
                                ctx.fillStyle = '#fff';
                                ctx.font = 'bold 12px Inter, sans-serif';
                                ctx.fillText(dinnerOptions[i].substring(0, 15), radius - 10, 4);
                                ctx.restore();
                            }
                            
                            // Center circle
                            ctx.beginPath();
                            ctx.arc(centerX, centerY, 20, 0, 2 * Math.PI);
                            ctx.fillStyle = '#fff';
                            ctx.fill();
                            ctx.strokeStyle = '#9333EA';
                            ctx.lineWidth = 3;
                            ctx.stroke();
                        }
                        
                        let currentRotation = 0;
                        
                        function spinWheel() {
                            if (dinnerOptions.length === 0) {
                                document.getElementById('wheelResult').textContent = 'Add dinner options first!';
                                document.getElementById('wheelResult').classList.remove('hidden');
                                return;
                            }
                            
                            const canvas = document.getElementById('wheelCanvas');
                            const spins = 5 + Math.random() * 5;
                            const extraRotation = Math.random() * 2 * Math.PI;
                            const totalRotation = spins * 2 * Math.PI + extraRotation;
                            
                            let start = null;
                            const duration = 3000;
                            
                            function animate(timestamp) {
                                if (!start) start = timestamp;
                                const progress = (timestamp - start) / duration;
                                
                                if (progress < 1) {
                                    const easeOut = 1 - Math.pow(1 - progress, 3);
                                    currentRotation = easeOut * totalRotation;
                                    canvas.style.transform = 'rotate(' + currentRotation + 'rad)';
                                    requestAnimationFrame(animate);
                                } else {
                                    // Calculate winner
                                    const sliceAngle = (2 * Math.PI) / dinnerOptions.length;
                                    const normalizedRotation = totalRotation % (2 * Math.PI);
                                    const index = Math.floor(dinnerOptions.length - (normalizedRotation / sliceAngle)) % dinnerOptions.length;
                                    const winner = dinnerOptions[index >= 0 ? index : index + dinnerOptions.length];
                                    
                                    document.getElementById('wheelResult').innerHTML = '🎉 <span class="text-pink-600">' + winner + '</span> 🎉';
                                    document.getElementById('wheelResult').classList.remove('hidden');
                                }
                            }
                            
                            requestAnimationFrame(animate);
                        }
                        
                        drawWheel();
                    </script>
                    {{else}}
                    <p class="text-gray-500 text-sm mb-4">No dinner options yet. Add one below!</p>
                    {{end}}
                    
                    <form method="POST" action="/dinner/create" class="flex gap-2">
                        <input type="hidden" name="date" value="{{.CurrentDate}}">
                        <input type="text" name="option" placeholder="Add dinner option..." required
                            class="flex-1 px-3 py-2 text-sm border border-gray-300 rounded-lg focus:ring-2 focus:ring-orange-500 focus:border-orange-500 transition-colors">
                        <button type="submit" class="px-3 py-2 bg-orange-500 text-white text-sm rounded-lg hover:bg-orange-600 transition-colors">
                            Add
                        </button>
                    </form>
                </div>

                <div class="bg-white rounded-2xl shadow-lg p-6 border border-gray-100">
                    <h2 class="text-xl font-semibold text-gray-800 mb-4 flex items-center gap-2">
                        <svg class="w-5 h-5 text-indigo-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
                        </svg>
                        Add Item
                    </h2>
                    <form method="POST" action="/add" class="space-y-4">
                        <div>
                            <label class="block text-sm font-medium text-gray-700 mb-1">Date</label>
                            <input type="date" name="date" value="{{.CurrentDate}}" 
                                class="w-full px-4 py-2.5 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition-colors">
                        </div>
                        <div>
                            <label class="block text-sm font-medium text-gray-700 mb-1">Your Name</label>
                            <input type="text" name="first_name" list="name-list" placeholder="Enter your name" required
                                class="w-full px-4 py-2.5 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition-colors">
                            <datalist id="name-list">
                                {{range .Names}}
                                <option value="{{.}}">{{.}}</option>
                                {{end}}
                            </datalist>
                        </div>
                        <div>
                            <label class="block text-sm font-medium text-gray-700 mb-1">Item Needed</label>
                            <input type="text" name="item" placeholder="What do you need?" required
                                class="w-full px-4 py-2.5 border border-gray-300 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition-colors">
                        </div>
                        <button type="submit" 
                            class="w-full bg-gradient-to-r from-blue-500 to-indigo-600 text-white font-medium py-2.5 px-4 rounded-lg hover:from-blue-600 hover:to-indigo-700 focus:ring-4 focus:ring-indigo-200 transition-all shadow-md">
                            Add to List
                        </button>
                    </form>
                </div>

                <!-- Past Lists -->
                <div class="bg-white rounded-2xl shadow-lg p-6 border border-gray-100 mt-6">
                    <h2 class="text-xl font-semibold text-gray-800 mb-4 flex items-center gap-2">
                        <svg class="w-5 h-5 text-indigo-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                        </svg>
                        Past Lists
                    </h2>
                    {{if .Dates}}
                    <ul class="space-y-2">
                        {{range .Dates}}
                        <li>
                            <a href="/?date={{.Date}}" 
                                class="block px-4 py-2.5 rounded-lg bg-gray-50 hover:bg-indigo-50 text-gray-700 hover:text-indigo-700 transition-colors border border-gray-200 hover:border-indigo-200">
                                <span class="flex items-center gap-2">
                                    <svg class="w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"></path>
                                    </svg>
                                    {{.Date}}
                                </span>
                            </a>
                        </li>
                        {{end}}
                    </ul>
                    {{else}}
                    <p class="text-gray-500 text-sm">No history yet.</p>
                    {{end}}
                </div>
            </div>

            <!-- Right Column: Current List -->
            <div class="lg:col-span-2">
                <div class="bg-white rounded-2xl shadow-lg p-6 border border-gray-100">
                    <div class="flex items-center justify-between mb-4">
                        <h2 class="text-xl font-semibold text-gray-800 flex items-center gap-2">
                            <svg class="w-5 h-5 text-indigo-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"></path>
                            </svg>
                            Shopping List
                        </h2>
                        <span class="text-sm text-gray-500 bg-gray-100 px-3 py-1 rounded-full">{{.CurrentDate}}</span>
                    </div>

                    {{if .Items}}
                    <div class="overflow-hidden rounded-xl border border-gray-200">
                        <table class="w-full">
                            <thead class="bg-gray-50">
                                <tr>
                                    <th class="px-6 py-3 text-left text-xs font-semibold text-gray-600 uppercase tracking-wider">Time</th>
                                    <th class="px-6 py-3 text-left text-xs font-semibold text-gray-600 uppercase tracking-wider">Who</th>
                                    <th class="px-6 py-3 text-left text-xs font-semibold text-gray-600 uppercase tracking-wider">Item</th>
                                </tr>
                            </thead>
                            <tbody class="divide-y divide-gray-200">
                                {{range .Items}}
                                <tr class="hover:bg-gray-50 transition-colors">
                                    <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-600 font-mono">{{.CreatedAt.Format "15:04"}}</td>
                                    <td class="px-6 py-4 whitespace-nowrap">
                                        <span class="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium bg-indigo-100 text-indigo-800">
                                            {{.FirstName}}
                                        </span>
                                    </td>
                                    <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-800 font-medium">{{.Item}}</td>
                                </tr>
                                {{end}}
                            </tbody>
                        </table>
                    </div>
                    {{else}}
                    <div class="text-center py-12">
                        <svg class="w-16 h-16 mx-auto text-gray-300 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"></path>
                        </svg>
                        <p class="text-gray-500">No items yet for this date.</p>
                        <p class="text-sm text-gray-400 mt-1">Add your first item to get started!</p>
                    </div>
                    {{end}}
                </div>
            </div>
        </div>

        {{else if eq .PageType "news"}}
        <!-- Malta News Page -->
        <div class="max-w-4xl mx-auto">
            <div class="bg-white rounded-2xl shadow-lg p-6 border border-gray-100">
                <div class="flex items-center justify-between mb-6">
                    <h2 class="text-2xl font-bold text-gray-800 flex items-center gap-2">
                        <svg class="w-6 h-6 text-purple-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 20H5a2 2 0 01-2-2V6a2 2 0 012-2h10a2 2 0 012 2v1m2 13a2 2 0 01-2-2V7m2 13a2 2 0 002-2V9a2 2 0 00-2-2h-2m-4-3H9M7 16h6M7 8h6v4H7V8z"></path>
                        </svg>
                        Latest Malta News
                    </h2>
                    <span class="text-sm text-gray-500">{{len .NewsArticles}} articles</span>
                </div>

                {{if .NewsArticles}}
                <div class="space-y-4">
                    {{range .NewsArticles}}
                    <div class="border border-gray-200 rounded-lg p-4 hover:shadow-md transition-shadow bg-white hover:bg-purple-50">
                        <div class="flex items-start justify-between">
                            <div class="flex-1">
                                <h3 class="text-lg font-semibold text-gray-800 mb-2">
                                    <a href="{{.URL}}" target="_blank" class="hover:text-purple-600 transition-colors">
                                        {{.Title}}
                                    </a>
                                </h3>
                                <p class="text-gray-600 text-sm mb-3">{{.Description}}</p>
                                <div class="flex items-center gap-3 text-xs text-gray-500">
                                    <span class="inline-flex items-center px-2 py-1 bg-purple-100 text-purple-800 rounded-full font-medium">
                                        {{.Source}}
                                    </span>
                                    <span>
                                        <svg class="w-3 h-3 inline mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                                        </svg>
                                        {{.PublishedAt}}
                                    </span>
                                </div>
                            </div>
                            <div class="ml-4">
                                <a href="{{.URL}}" target="_blank" class="inline-flex items-center justify-center w-10 h-10 bg-purple-500 text-white rounded-full hover:bg-purple-600 transition-colors">
                                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
                                    </svg>
                                </a>
                            </div>
                        </div>
                    </div>
                    {{end}}
                </div>
                {{else}}
                <div class="text-center py-12">
                    <svg class="w-16 h-16 text-gray-300 mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 20H5a2 2 0 01-2-2V6a2 2 0 012-2h10a2 2 0 012 2v1m2 13a2 2 0 01-2-2V7m2 13a2 2 0 002-2V9a2 2 0 00-2-2h-2m-4-3H9M7 16h6M7 8h6v4H7V8z"></path>
                    </svg>
                    <p class="text-gray-500">No news articles available at the moment.</p>
                    <p class="text-gray-400 text-sm mt-2">Please check back later for the latest Malta news.</p>
                </div>
                {{end}}
            </div>

            <!-- News Sources Info -->
            <div class="mt-6 bg-purple-50 rounded-2xl p-4 border border-purple-100">
                <h3 class="text-sm font-semibold text-purple-800 mb-2">🇲🇹 Malta News Sources</h3>
                <p class="text-xs text-purple-600">
                    This page aggregates news from leading Maltese media outlets including Times of Malta, Malta Independent, 
                    MaltaToday, Lovin Malta, and Newsbook Malta to keep you updated with the latest happenings in Malta.
                </p>
            </div>
        </div>
        {{else if eq .PageType "analytics"}}
        <!-- Analytics Page -->
        <div class="max-w-6xl mx-auto">
            <!-- Shopping Patterns -->
            <div class="bg-white rounded-2xl shadow-lg p-6 border border-gray-100 mb-6">
                <h2 class="text-xl font-semibold text-gray-800 mb-4 flex items-center gap-2">
                    <svg class="w-5 h-5 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 11V7a4 4 0 00-8 0v4M5 9h14l1 12H4L5 9z"></path>
                    </svg>
                    Shopping Patterns
                </h2>
                <div class="grid md:grid-cols-3 gap-4 mb-4">
                    <div class="bg-blue-50 rounded-lg p-4">
                        <p class="text-2xl font-bold text-blue-600">{{.Analytics.ShoppingPatterns.TotalItems}}</p>
                        <p class="text-sm text-gray-600">Total Items</p>
                    </div>
                    <div class="bg-green-50 rounded-lg p-4">
                        <p class="text-2xl font-bold text-green-600">{{.Analytics.ShoppingPatterns.UniqueItems}}</p>
                        <p class="text-sm text-gray-600">Unique Items</p>
                    </div>
                    <div class="bg-purple-50 rounded-lg p-4">
                        <p class="text-2xl font-bold text-purple-600">{{.Analytics.ShoppingPatterns.MostActiveDay}}</p>
                        <p class="text-sm text-gray-600">Most Active Day</p>
                    </div>
                </div>
                {{if .Analytics.ShoppingPatterns.TopItems}}
                <div class="mt-4">
                    <h3 class="text-sm font-semibold text-gray-700 mb-2">Top Items</h3>
                    <div class="space-y-2">
                        {{range .Analytics.ShoppingPatterns.TopItems}}
                        <div class="flex items-center justify-between p-2 bg-gray-50 rounded-lg">
                            <span class="text-sm font-medium">{{.Item}}</span>
                            <div class="flex items-center gap-2">
                                <span class="text-xs text-gray-500">{{.LastUsed}}</span>
                                <span class="px-2 py-1 bg-blue-100 text-blue-800 text-xs rounded-full">{{.Count}}x</span>
                            </div>
                        </div>
                        {{end}}
                    </div>
                </div>
                {{end}}
            </div>

            <!-- Chore Performance -->
            <div class="bg-white rounded-2xl shadow-lg p-6 border border-gray-100 mb-6">
                <h2 class="text-xl font-semibold text-gray-800 mb-4 flex items-center gap-2">
                    <svg class="w-5 h-5 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                    </svg>
                    Chore Performance
                </h2>
                <div class="grid md:grid-cols-3 gap-4 mb-4">
                    <div class="bg-green-50 rounded-lg p-4">
                        <p class="text-2xl font-bold text-green-600">{{.Analytics.ChorePerformance.TotalChores}}</p>
                        <p class="text-sm text-gray-600">Total Chores</p>
                    </div>
                    <div class="bg-blue-50 rounded-lg p-4">
                        <p class="text-2xl font-bold text-blue-600">{{.Analytics.ChorePerformance.CompletedChores}}</p>
                        <p class="text-sm text-gray-600">Completed</p>
                    </div>
                    <div class="bg-orange-50 rounded-lg p-4">
                        <p class="text-2xl font-bold text-orange-600">{{printf "%.1f" .Analytics.ChorePerformance.CompletionRate}}%</p>
                        <p class="text-sm text-gray-600">Completion Rate</p>
                    </div>
                </div>
                {{if .Analytics.ChorePerformance.TopPerformers}}
                <div class="mt-4">
                    <h3 class="text-sm font-semibold text-gray-700 mb-2">Top Performers</h3>
                    <div class="space-y-2">
                        {{range .Analytics.ChorePerformance.TopPerformers}}
                        <div class="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                            <div>
                                <p class="text-sm font-medium">{{.Name}}</p>
                                <p class="text-xs text-gray-500">{{.Chores}} chores • {{printf "%.1f" .Rate}}% completion</p>
                            </div>
                            <div class="text-right">
                                <p class="text-lg font-bold text-green-600">{{.Points}}</p>
                                <p class="text-xs text-gray-500">points</p>
                            </div>
                        </div>
                        {{end}}
                    </div>
                </div>
                {{end}}
            </div>

            <!-- Family Stats -->
            <div class="bg-white rounded-2xl shadow-lg p-6 border border-gray-100 mb-6">
                <h2 class="text-xl font-semibold text-gray-800 mb-4 flex items-center gap-2">
                    <svg class="w-5 h-5 text-purple-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z"></path>
                    </svg>
                    Family Statistics
                </h2>
                <div class="grid md:grid-cols-2 gap-4 mb-4">
                    <div class="bg-orange-50 rounded-lg p-4">
                        <p class="text-2xl font-bold text-orange-600">{{.Analytics.FamilyStats.TotalPoints}}</p>
                        <p class="text-sm text-gray-600">Total Points Earned</p>
                    </div>
                    <div class="bg-purple-50 rounded-lg p-4">
                        <p class="text-2xl font-bold text-purple-600">{{len .Analytics.FamilyStats.ActiveMembers}}</p>
                        <p class="text-sm text-gray-600">Active Members</p>
                    </div>
                </div>
                {{if .Analytics.FamilyStats.ActiveMembers}}
                <div class="mt-4">
                    <h3 class="text-sm font-semibold text-gray-700 mb-2">Active Family Members</h3>
                    <div class="flex flex-wrap gap-2">
                        {{range .Analytics.FamilyStats.ActiveMembers}}
                        <span class="px-3 py-1 bg-purple-100 text-purple-800 text-sm rounded-full font-medium">{{.}}</span>
                        {{end}}
                    </div>
                </div>
                {{end}}
            </div>

            <!-- Weekly Activity -->
            <div class="bg-white rounded-2xl shadow-lg p-6 border border-gray-100">
                <h2 class="text-xl font-semibold text-gray-800 mb-4 flex items-center gap-2">
                    <svg class="w-5 h-5 text-indigo-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"></path>
                    </svg>
                    Weekly Activity
                </h2>
                {{if .Analytics.FamilyStats.WeeklyActivity}}
                <div class="space-y-2">
                    {{range .Analytics.FamilyStats.WeeklyActivity}}
                    <div class="flex items-center justify-between p-2 bg-gray-50 rounded-lg">
                        <span class="text-sm font-medium">{{.Date}}</span>
                        <div class="flex items-center gap-4">
                            <span class="text-xs text-blue-600">{{.Items}} items</span>
                            <span class="text-xs text-green-600">{{.Chores}} chores</span>
                        </div>
                    </div>
                    {{end}}
                </div>
                {{else}}
                <p class="text-gray-500 text-center py-4">No activity data available yet.</p>
                {{end}}
            </div>
        </div>
        {{else if eq .PageType "pacman"}}
        <!-- Pac-Man Game Page -->
        <div class="max-w-4xl mx-auto">
            <div class="bg-black rounded-2xl shadow-2xl p-6 border-4 border-yellow-400">
                <!-- Game Header -->
                <div class="flex items-center justify-between mb-4">
                    <div class="flex items-center gap-4">
                        <div class="text-yellow-400 font-bold text-xl">SCORE: <span id="score">0</span></div>
                        <div class="text-yellow-400 font-bold text-xl">LIVES: <span id="lives">3</span></div>
                    </div>
                    <div class="flex gap-2">
                        <button id="startBtn" class="px-4 py-2 bg-green-600 text-white font-bold rounded hover:bg-green-700 transition-colors">
                            START GAME
                        </button>
                        <button id="pauseBtn" class="px-4 py-2 bg-yellow-600 text-white font-bold rounded hover:bg-yellow-700 transition-colors">
                            PAUSE
                        </button>
                    </div>
                </div>

                <!-- Game Canvas -->
                <div class="relative bg-black rounded-lg overflow-hidden">
                    <canvas id="gameCanvas" width="600" height="400" class="w-full border-2 border-blue-600"></canvas>
                    
                    <!-- Game Over Overlay -->
                    <div id="gameOverlay" class="absolute inset-0 bg-black bg-opacity-80 flex items-center justify-center hidden">
                        <div class="text-center">
                            <h2 id="gameTitle" class="text-4xl font-bold text-yellow-400 mb-4">GAME OVER</h2>
                            <p id="gameMessage" class="text-white text-xl mb-4">Final Score: 0</p>
                            <button id="restartBtn" class="px-6 py-3 bg-green-600 text-white font-bold rounded-lg hover:bg-green-700 transition-colors">
                                PLAY AGAIN
                            </button>
                        </div>
                    </div>
                </div>

                <!-- Controls Info -->
                <div class="mt-4 grid md:grid-cols-2 gap-4">
                    <div class="bg-gray-900 rounded-lg p-4 border border-gray-700">
                        <h3 class="text-yellow-400 font-bold mb-2">🎮 Controls</h3>
                        <div class="text-gray-300 text-sm space-y-1">
                            <p>↑ ↓ ← → Arrow Keys or WASD to move</p>
                            <p>SPACE to pause/resume</p>
                            <p>Touch controls on mobile devices</p>
                        </div>
                    </div>
                    <div class="bg-gray-900 rounded-lg p-4 border border-gray-700">
                        <h3 class="text-yellow-400 font-bold mb-2">🎯 Objectives</h3>
                        <div class="text-gray-300 text-sm space-y-1">
                            <p>🟡 Eat all dots to advance</p>
                            <p>🍒 Grab cherries for bonus points</p>
                            <p>👻 Avoid ghosts or lose a life</p>
                            <p>⭐ Get high scores!</p>
                        </div>
                    </div>
                </div>

                <!-- High Scores -->
                <div class="mt-4 bg-gray-900 rounded-lg p-4 border border-gray-700">
                    <h3 class="text-yellow-400 font-bold mb-2">🏆 High Scores</h3>
                    <div id="highScores" class="text-gray-300 text-sm space-y-1">
                        <div class="flex justify-between">
                            <span>1. Player</span>
                            <span>0</span>
                        </div>
                        <div class="flex justify-between">
                            <span>2. Player</span>
                            <span>0</span>
                        </div>
                        <div class="flex justify-between">
                            <span>3. Player</span>
                            <span>0</span>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Mobile Touch Controls -->
            <div class="md:hidden mt-4 grid grid-cols-3 gap-2 max-w-xs mx-auto">
                <div></div>
                <button class="touch-control bg-gray-800 text-white p-4 rounded-lg" data-direction="up">↑</button>
                <div></div>
                <button class="touch-control bg-gray-800 text-white p-4 rounded-lg" data-direction="left">←</button>
                <button class="touch-control bg-gray-800 text-white p-4 rounded-lg" data-direction="down">↓</button>
                <button class="touch-control bg-gray-800 text-white p-4 rounded-lg" data-direction="right">→</button>
            </div>
        </div>
        {{end}}

        <!-- Footer -->
        <footer class="text-center mt-8 text-gray-600 text-sm">
            <div class="flex items-center justify-center gap-2">
                <!-- AI Brain Logo -->
                <svg class="w-5 h-5 text-purple-600" fill="currentColor" viewBox="0 0 24 24">
                    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8z" fill="none" stroke="currentColor" stroke-width="1.5"/>
                    <circle cx="9" cy="9" r="1.5" fill="currentColor"/>
                    <circle cx="15" cy="9" r="1.5" fill="currentColor"/>
                    <circle cx="12" cy="15" r="1.5" fill="currentColor"/>
                    <path d="M9 10.5c.83 0 1.5-.67 1.5-1.5S9.83 7.5 9 7.5 7.5 8.17 7.5 9s.67 1.5 1.5 1.5zm6 0c.83 0 1.5-.67 1.5-1.5s-.67-1.5-1.5-1.5-1.5.67-1.5 1.5.67 1.5 1.5 1.5zm-3 4c.83 0 1.5-.67 1.5-1.5s-.67-1.5-1.5-1.5-1.5.67-1.5 1.5.67 1.5 1.5 1.5z" fill="white"/>
                    <path d="M12 7c.55 0 1-.45 1-1s-.45-1-1-1-1 .45-1 1 .45 1 1 1zm0 10c.55 0 1-.45 1-1s-.45-1-1-1-1 .45-1 1 .45 1 1 1zm5-5c.55 0 1-.45 1-1s-.45-1-1-1-1 .45-1 1 .45 1 1 1zm-10 0c.55 0 1-.45 1-1s-.45-1-1-1-1 .45-1 1 .45 1 1 1z" fill="currentColor" opacity="0.6"/>
                </svg>
                <span class="font-medium text-purple-700">Brain Emerge</span>
            </div>
            <p class="text-xs text-gray-500 mt-1">AI-Powered Family Solutions</p>
        </footer>
    </div>
</body>
{{if eq .PageType "pacman"}}
        // Pac-Man Game JavaScript
        <script>
        class PacManGame {
            constructor() {
                this.canvas = document.getElementById('gameCanvas');
                this.ctx = this.canvas.getContext('2d');
                this.score = 0;
                this.lives = 3;
                this.gameRunning = false;
                this.gamePaused = false;
                this.highScores = this.loadHighScores();
                
                // Game entities
                this.pacman = { x: 300, y: 200, size: 15, speed: 2, direction: 'right' };
                this.ghosts = [];
                this.dots = [];
                this.powerPellets = [];
                this.cherries = [];
                
                // Maze layout (simplified)
                this.maze = this.generateMaze();
                
                this.init();
            }
            
            init() {
                this.setupEventListeners();
                this.generateDots();
                this.generateGhosts();
                this.updateUI();
            }
            
            setupEventListeners() {
                // Keyboard controls
                document.addEventListener('keydown', (e) => this.handleKeyPress(e));
                
                // Button controls
                document.getElementById('startBtn').addEventListener('click', () => this.startGame());
                document.getElementById('pauseBtn').addEventListener('click', () => this.togglePause());
                document.getElementById('restartBtn').addEventListener('click', () => this.restartGame());
                
                // Touch controls
                document.querySelectorAll('.touch-control').forEach(btn => {
                    btn.addEventListener('click', (e) => {
                        const direction = e.target.dataset.direction;
                        this.changeDirection(direction);
                    });
                });
            }
            
            handleKeyPress(e) {
                if (!this.gameRunning || this.gamePaused) return;
                
                switch(e.key.toLowerCase()) {
                    case 'arrowup':
                    case 'w':
                        this.changeDirection('up');
                        break;
                    case 'arrowdown':
                    case 's':
                        this.changeDirection('down');
                        break;
                    case 'arrowleft':
                    case 'a':
                        this.changeDirection('left');
                        break;
                    case 'arrowright':
                    case 'd':
                        this.changeDirection('right');
                        break;
                    case ' ':
                        this.togglePause();
                        break;
                }
            }
            
            changeDirection(direction) {
                this.pacman.direction = direction;
            }
            
            generateMaze() {
                // Simple maze representation
                const maze = [];
                for (let y = 0; y < 20; y++) {
                    maze[y] = [];
                    for (let x = 0; x < 30; x++) {
                        maze[y][x] = 0; // 0 = empty, 1 = wall
                    }
                }
                
                // Add some walls
                for (let i = 5; i < 10; i++) {
                    maze[5][i] = 1;
                    maze[14][i] = 1;
                    maze[i][5] = 1;
                    maze[i][24] = 1;
                }
                
                return maze;
            }
            
            generateDots() {
                this.dots = [];
                for (let y = 0; y < 20; y++) {
                    for (let x = 0; x < 30; x++) {
                        if (this.maze[y][x] === 0 && Math.random() > 0.7) {
                            this.dots.push({ x: x * 20, y: y * 20, size: 3 });
                        }
                    }
                }
            }
            
            generateGhosts() {
                this.ghosts = [
                    { x: 100, y: 100, size: 15, speed: 1.5, color: '#ff0000', direction: 'right' },
                    { x: 500, y: 100, size: 15, speed: 1.5, color: '#00ffff', direction: 'left' },
                    { x: 100, y: 300, size: 15, speed: 1.5, color: '#ffb8ff', direction: 'right' },
                    { x: 500, y: 300, size: 15, speed: 1.5, color: '#ffb852', direction: 'left' }
                ];
            }
            
            startGame() {
                this.gameRunning = true;
                this.gamePaused = false;
                this.score = 0;
                this.lives = 3;
                this.pacman = { x: 300, y: 200, size: 15, speed: 2, direction: 'right' };
                this.generateDots();
                this.generateGhosts();
                this.hideOverlay();
                this.gameLoop();
            }
            
            togglePause() {
                if (!this.gameRunning) return;
                this.gamePaused = !this.gamePaused;
                if (!this.gamePaused) {
                    this.gameLoop();
                }
            }
            
            restartGame() {
                this.startGame();
            }
            
            gameLoop() {
                if (!this.gameRunning || this.gamePaused) return;
                
                this.update();
                this.render();
                this.checkCollisions();
                
                requestAnimationFrame(() => this.gameLoop());
            }
            
            update() {
                // Update Pac-Man position
                this.movePacman();
                
                // Update ghosts
                this.moveGhosts();
                
                // Update UI
                this.updateUI();
            }
            
            movePacman() {
                const speed = this.pacman.speed;
                switch(this.pacman.direction) {
                    case 'up':
                        this.pacman.y -= speed;
                        break;
                    case 'down':
                        this.pacman.y += speed;
                        break;
                    case 'left':
                        this.pacman.x -= speed;
                        break;
                    case 'right':
                        this.pacman.x += speed;
                        break;
                }
                
                // Keep Pac-Man in bounds
                this.pacman.x = Math.max(this.pacman.size, Math.min(this.canvas.width - this.pacman.size, this.pacman.x));
                this.pacman.y = Math.max(this.pacman.size, Math.min(this.canvas.height - this.pacman.size, this.pacman.y));
            }
            
            moveGhosts() {
                this.ghosts.forEach(ghost => {
                    // Simple AI: move towards Pac-Man sometimes, random other times
                    if (Math.random() > 0.5) {
                        // Move towards Pac-Man
                        if (ghost.x < this.pacman.x) ghost.x += ghost.speed;
                        else if (ghost.x > this.pacman.x) ghost.x -= ghost.speed;
                        
                        if (ghost.y < this.pacman.y) ghost.y += ghost.speed;
                        else if (ghost.y > this.pacman.y) ghost.y -= ghost.speed;
                    } else {
                        // Random movement
                        const directions = ['up', 'down', 'left', 'right'];
                        const dir = directions[Math.floor(Math.random() * 4)];
                        
                        switch(dir) {
                            case 'up': ghost.y -= ghost.speed; break;
                            case 'down': ghost.y += ghost.speed; break;
                            case 'left': ghost.x -= ghost.speed; break;
                            case 'right': ghost.x += ghost.speed; break;
                        }
                    }
                    
                    // Keep ghosts in bounds
                    ghost.x = Math.max(ghost.size, Math.min(this.canvas.width - ghost.size, ghost.x));
                    ghost.y = Math.max(ghost.size, Math.min(this.canvas.height - ghost.size, ghost.y));
                });
            }
            
            checkCollisions() {
                // Check dot collection
                this.dots = this.dots.filter(dot => {
                    const distance = Math.sqrt(
                        Math.pow(this.pacman.x - dot.x, 2) + 
                        Math.pow(this.pacman.y - dot.y, 2)
                    );
                    
                    if (distance < this.pacman.size + dot.size) {
                        this.score += 10;
                        return false; // Remove dot
                    }
                    return true; // Keep dot
                });
                
                // Check ghost collision
                this.ghosts.forEach(ghost => {
                    const distance = Math.sqrt(
                        Math.pow(this.pacman.x - ghost.x, 2) + 
                        Math.pow(this.pacman.y - ghost.y, 2)
                    );
                    
                    if (distance < this.pacman.size + ghost.size) {
                        this.loseLife();
                    }
                });
                
                // Check win condition
                if (this.dots.length === 0) {
                    this.nextLevel();
                }
            }
            
            loseLife() {
                this.lives--;
                if (this.lives <= 0) {
                    this.gameOver();
                } else {
                    // Reset positions
                    this.pacman = { x: 300, y: 200, size: 15, speed: 2, direction: 'right' };
                }
            }
            
            nextLevel() {
                this.score += 100;
                this.generateDots();
                this.generateGhosts();
                // Increase difficulty
                this.ghosts.forEach(ghost => ghost.speed *= 1.1);
            }
            
            gameOver() {
                this.gameRunning = false;
                this.saveHighScore();
                this.showOverlay('GAME OVER', 'Final Score: ' + this.score);
            }
            
            render() {
                // Clear canvas
                this.ctx.fillStyle = '#000000';
                this.ctx.fillRect(0, 0, this.canvas.width, this.canvas.height);
                
                // Draw maze (simplified)
                this.ctx.fillStyle = '#0000ff';
                for (let y = 0; y < 20; y++) {
                    for (let x = 0; x < 30; x++) {
                        if (this.maze[y][x] === 1) {
                            this.ctx.fillRect(x * 20, y * 20, 20, 20);
                        }
                    }
                }
                
                // Draw dots
                this.ctx.fillStyle = '#ffffff';
                this.dots.forEach(dot => {
                    this.ctx.beginPath();
                    this.ctx.arc(dot.x, dot.y, dot.size, 0, Math.PI * 2);
                    this.ctx.fill();
                });
                
                // Draw Pac-Man
                this.ctx.fillStyle = '#ffff00';
                this.ctx.beginPath();
                this.ctx.arc(this.pacman.x, this.pacman.y, this.pacman.size, 0.2 * Math.PI, 1.8 * Math.PI);
                this.ctx.lineTo(this.pacman.x, this.pacman.y);
                this.ctx.fill();
                
                // Draw ghosts
                this.ghosts.forEach(ghost => {
                    this.ctx.fillStyle = ghost.color;
                    this.ctx.beginPath();
                    this.ctx.arc(ghost.x, ghost.y, ghost.size, Math.PI, 0);
                    this.ctx.lineTo(ghost.x + ghost.size, ghost.y + ghost.size);
                    this.ctx.lineTo(ghost.x - ghost.size, ghost.y + ghost.size);
                    this.ctx.closePath();
                    this.ctx.fill();
                    
                    // Ghost eyes
                    this.ctx.fillStyle = '#ffffff';
                    this.ctx.beginPath();
                    this.ctx.arc(ghost.x - 5, ghost.y - 5, 3, 0, Math.PI * 2);
                    this.ctx.arc(ghost.x + 5, ghost.y - 5, 3, 0, Math.PI * 2);
                    this.ctx.fill();
                });
            }
            
            updateUI() {
                document.getElementById('score').textContent = this.score;
                document.getElementById('lives').textContent = this.lives;
            }
            
            showOverlay(title, message) {
                document.getElementById('gameTitle').textContent = title;
                document.getElementById('gameMessage').textContent = message;
                document.getElementById('gameOverlay').classList.remove('hidden');
            }
            
            hideOverlay() {
                document.getElementById('gameOverlay').classList.add('hidden');
            }
            
            loadHighScores() {
                const scores = localStorage.getItem('pacmanHighScores');
                return scores ? JSON.parse(scores) : [];
            }
            
            saveHighScore() {
                this.highScores.push({ score: this.score, date: new Date().toLocaleDateString() });
                this.highScores.sort((a, b) => b.score - a.score);
                this.highScores = this.highScores.slice(0, 5);
                localStorage.setItem('pacmanHighScores', JSON.stringify(this.highScores));
                this.updateHighScoresDisplay();
            }
            
            updateHighScoresDisplay() {
                const scoresContainer = document.getElementById('highScores');
                scoresContainer.innerHTML = this.highScores.map((score, index) => 
                    '<div class="flex justify-between">' +
                        '<span>' + (index + 1) + '. Player</span>' +
                        '<span>' + score.score + '</span>' +
                    '</div>'
                ).join('');
            }
        }

        // Initialize game when page loads
        document.addEventListener('DOMContentLoaded', () => {
            new PacManGame();
        });
        </script>
        {{end}}
        </body>
        </html>
        `
