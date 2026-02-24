package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Item struct {
	FirstName string
	Item      string
	CreatedAt time.Time
}

type ListDate struct {
	Date string
}

type PageData struct {
	CurrentDate string
	Items       []Item
	Dates       []ListDate
	Message     string
}

var (
	db   *pgxpool.Pool
	tmpl *template.Template
	ctx  = context.Background()
)

func main() {
	// DATABASE_URL must be set (Render will set it for the managed Postgres)
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	var err error
	db, err = pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := migrate(); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	tmpl = template.Must(template.New("index").Parse(htmlTemplate))

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/add", handleAdd)
	http.HandleFunc("/history", handleHistory)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func migrate() error {
	_, err := db.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS lists (
            id SERIAL PRIMARY KEY,
            list_date DATE NOT NULL UNIQUE
        );

        CREATE TABLE IF NOT EXISTS items (
            id SERIAL PRIMARY KEY,
            list_id INT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
            first_name TEXT NOT NULL,
            item TEXT NOT NULL,
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
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

	data := PageData{
		CurrentDate: dateStr,
		Items:       items,
		Dates:       dates,
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

	data := PageData{
		Dates: dates,
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func getOrCreateListID(dateStr string) (int, error) {
	var id int
	err := db.QueryRow(ctx, `
        INSERT INTO lists (list_date)
        VALUES ($1)
        ON CONFLICT (list_date) DO UPDATE SET list_date = EXCLUDED.list_date
        RETURNING id;
    `, dateStr).Scan(&id)
	return id, err
}

func addItem(firstName, item, dateStr string) error {
	listID, err := getOrCreateListID(dateStr)
	if err != nil {
		return err
	}

	_, err = db.Exec(ctx, `
        INSERT INTO items (list_id, first_name, item)
        VALUES ($1, $2, $3);
    `, listID, firstName, item)
	return err
}

func getItemsByDate(dateStr string) ([]Item, error) {
	rows, err := db.Query(ctx, `
        SELECT i.first_name, i.item, i.created_at
        FROM items i
        JOIN lists l ON i.list_id = l.id
        WHERE l.list_date = $1
        ORDER BY i.created_at ASC;
    `, dateStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.FirstName, &it.Item, &it.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func getAllDates() ([]ListDate, error) {
	rows, err := db.Query(ctx, `
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
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		dates = append(dates, ListDate{Date: d.Format("2006-01-02")})
	}
	return dates, rows.Err()
}

const htmlTemplate = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Family Shopping List</title>
    <style>
        body { font-family: sans-serif; max-width: 700px; margin: 20px auto; }
        h1, h2 { margin-bottom: 0.3em; }
        form { margin-bottom: 1em; }
        label { display: block; margin-top: 0.5em; }
        input[type="text"], input[type="date"] { width: 100%; padding: 0.4em; }
        button { margin-top: 0.7em; padding: 0.4em 0.8em; }
        table { width: 100%; border-collapse: collapse; margin-top: 1em; }
        th, td { border-bottom: 1px solid #ddd; padding: 0.4em; text-align: left; }
        .date-list a { text-decoration: none; }
    </style>
</head>
<body>
    <h1>Family Shopping List</h1>

    <h2>Add item</h2>
    <form method="POST" action="/add">
        <label>
            Date:
            <input type="date" name="date" value="{{.CurrentDate}}">
        </label>
        <label>
            First name:
            <input type="text" name="first_name" required>
        </label>
        <label>
            Item needed:
            <input type="text" name="item" required>
        </label>
        <button type="submit">Add</button>
    </form>

    <h2>Items for {{.CurrentDate}}</h2>
    {{if .Items}}
    <table>
        <tr>
            <th>Time</th>
            <th>Who</th>
            <th>Item</th>
        </tr>
        {{range .Items}}
        <tr>
            <td>{{.CreatedAt.Format "15:04"}}</td>
            <td>{{.FirstName}}</td>
            <td>{{.Item}}</td>
        </tr>
        {{end}}
    </table>
    {{else}}
    <p>No items yet for this date.</p>
    {{end}}

    <h2>Past lists</h2>
    <ul class="date-list">
        {{range .Dates}}
        <li><a href="/?date={{.Date}}">{{.Date}}</a></li>
        {{else}}
        <li>No history yet.</li>
        {{end}}
    </ul>
</body>
</html>
`
