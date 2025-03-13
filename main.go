package main

import (
    "database/sql"
    "encoding/json"
    "io"
    "log"
    "net/http"
    
    _ "github.com/mattn/go-sqlite3"
)

type RequestLog struct {
    Method      string            `json:"method"`
    Path        string            `json:"path"`
    Headers     map[string]string `json:"headers"`
    Body        string            `json:"body"`
}

var db *sql.DB

func init() {
    var err error
    db, err = sql.Open("sqlite3", "./requests.db")
    if err != nil {
        log.Fatal(err)
    }

    // Create table if it doesn't exist
    createTable := `
    CREATE TABLE IF NOT EXISTS request_logs (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        method TEXT,
        path TEXT,
        headers TEXT,
        body TEXT,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );`

    _, err = db.Exec(createTable)
    if err != nil {
        log.Fatal(err)
    }
}

func logRequest(w http.ResponseWriter, r *http.Request) {
    // Read the request body
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Error reading request body", http.StatusInternalServerError)
        return
    }
    defer r.Body.Close()

    // Convert headers to map
    headers := make(map[string]string)
    for name, values := range r.Header {
        headers[name] = values[0]
    }

    // Convert headers to JSON string
    headersJSON, err := json.Marshal(headers)
    if err != nil {
        http.Error(w, "Error processing headers", http.StatusInternalServerError)
        return
    }

    // Store in database
    _, err = db.Exec(`
        INSERT INTO request_logs (method, path, headers, body)
        VALUES (?, ?, ?, ?)
    `, r.Method, r.URL.Path, string(headersJSON), string(body))

    if err != nil {
        http.Error(w, "Error storing request", http.StatusInternalServerError)
        return
    }

    // Send response
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("Request logged successfully"))
}

func main() {
    // Handle all requests
    http.HandleFunc("/", logRequest)

    log.Println("Server starting on :8080...")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        log.Fatal(err)
    }
}