package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Bin struct {
	BinID   string `json:"binId"`
	Now     int64  `json:"now"`
	Expires int64  `json:"expires"`
}

// Create a response struct that includes the count
type BinResponse struct {
	BinID   string `json:"binId"`
	Now     int64  `json:"now"`
	Expires int64  `json:"expires"`
	Entries int    `json:"entries"`
}

type Request struct {
	Method   string            `json:"method"`
	Path     string            `json:"path"`
	Headers  map[string]string `json:"headers"`
	Query    map[string]string `json:"query"`
	Body     interface{}       `json:"body"`
	IP       string            `json:"ip"`
	BinID    string            `json:"binId"`
	ReqID    string            `json:"reqId"`
	Inserted int64             `json:"inserted"`
}

var db *sql.DB

func generateID() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func init() {
	var err error
	db, err = sql.Open("sqlite3", "./postbin.db")
	if err != nil {
		log.Fatal(err)
	}

	// Create tables
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS bins (
            bin_id TEXT PRIMARY KEY,
            created_at INTEGER,
            expires_at INTEGER
        );
        CREATE TABLE IF NOT EXISTS requests (
            req_id TEXT PRIMARY KEY,
            bin_id TEXT,
            method TEXT,
            path TEXT,
            headers TEXT,
            query TEXT,
            body TEXT,
            ip TEXT,
            inserted INTEGER,
            FOREIGN KEY(bin_id) REFERENCES bins(bin_id)
        );
    `)
	if err != nil {
		log.Fatal(err)
	}
}

func createBinHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	binID := generateID()
	now := time.Now().UnixMilli()
	expires := now + (30 * 60 * 1000) // 30 minutes

	_, err := db.Exec("INSERT INTO bins (bin_id, created_at, expires_at) VALUES (?, ?, ?)",
		binID, now, expires)
	if err != nil {
		http.Error(w, `{"msg":"Internal Server Error"}`, http.StatusInternalServerError)
		return
	}

	// Create response with entries count (will be 0 for new bin)
	response := BinResponse{
		BinID:   binID,
		Now:     now,
		Expires: expires,
		Entries: 0,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func getBinHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	binID := r.URL.Path[len("/api/bin/"):]
	var bin Bin
	err := db.QueryRow("SELECT bin_id, created_at, expires_at FROM bins WHERE bin_id = ?", binID).
		Scan(&bin.BinID, &bin.Now, &bin.Expires)

	if err == sql.ErrNoRows {
		http.Error(w, `{"msg":"No such bin"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"msg":"Internal Server Error"}`, http.StatusInternalServerError)
		return
	}

	// Get the count of entries for this bin
	var entries int
	err = db.QueryRow("SELECT COUNT(*) FROM requests WHERE bin_id = ?", binID).Scan(&entries)
	if err != nil {
		http.Error(w, `{"msg":"Internal Server Error"}`, http.StatusInternalServerError)
		return
	}

	// Create response with entries count
	response := BinResponse{
		BinID:   bin.BinID,
		Now:     bin.Now,
		Expires: bin.Expires,
		Entries: entries,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func deleteBinHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	binID := r.URL.Path[len("/api/bin/"):]
	_, err := db.Exec("DELETE FROM bins WHERE bin_id = ?", binID)
	if err != nil {
		http.Error(w, `{"msg":"Internal Server Error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"msg":"Bin Deleted"}`)
}

func captureRequestHandler(w http.ResponseWriter, r *http.Request) {
	binID := r.URL.Path[1:] // Remove leading slash

	// Check if bin exists and not expired
	var expires int64
	err := db.QueryRow("SELECT expires_at FROM bins WHERE bin_id = ?", binID).Scan(&expires)
	if err == sql.ErrNoRows {
		http.Error(w, "Bin not found", http.StatusNotFound)
		return
	}
	if time.Now().UnixMilli() > expires {
		http.Error(w, "Bin expired", http.StatusGone)
		return
	}

	// Read and store request
	body, _ := io.ReadAll(r.Body)
	headers := make(map[string]string)
	for name, values := range r.Header {
		headers[name] = values[0]
	}
	query := make(map[string]string)
	for key, values := range r.URL.Query() {
		query[key] = values[0]
	}

	reqID := generateID()
	headersJSON, _ := json.Marshal(headers)
	queryJSON, _ := json.Marshal(query)
	bodyJSON, _ := json.Marshal(string(body))

	_, err = db.Exec(`
        INSERT INTO requests (req_id, bin_id, method, path, headers, query, body, ip, inserted)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		reqID, binID, r.Method, r.URL.Path, string(headersJSON), string(queryJSON), string(bodyJSON),
		r.RemoteAddr, time.Now().UnixMilli())
	if err != nil {
		http.Error(w, "Error storing request", http.StatusInternalServerError)
		return
	}

	w.Write([]byte(reqID))
}

func getRequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Path[len("/api/bin/"):]
	parts := strings.Split(path, "/req/")
	if len(parts) != 2 {
		http.Error(w, `{"msg":"Invalid path format"}`, http.StatusBadRequest)
		return
	}
	binID := parts[0]
	reqID := parts[1]

	var req Request
	var headersStr, queryStr, bodyStr string
	err := db.QueryRow(`
        SELECT method, path, headers, query, body, ip, bin_id, req_id, inserted
        FROM requests WHERE bin_id = ? AND req_id = ?`, binID, reqID).
		Scan(&req.Method, &req.Path, &headersStr, &queryStr, &bodyStr, &req.IP, &req.BinID,
			&req.ReqID, &req.Inserted)

	if err == sql.ErrNoRows {
		http.Error(w, `{"msg":"Request not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"msg":"Internal Server Error"}`, http.StatusInternalServerError)
		return
	}

	json.Unmarshal([]byte(headersStr), &req.Headers)
	json.Unmarshal([]byte(queryStr), &req.Query)
	json.Unmarshal([]byte(bodyStr), &req.Body)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(req)
}

func shiftRequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	binID := r.URL.Path[len("/api/bin/") : len(r.URL.Path)-len("/req/shift")]

	var req Request
	var headersStr, queryStr, bodyStr string
	err := db.QueryRow(`
        SELECT method, path, headers, query, body, ip, bin_id, req_id, inserted
        FROM requests WHERE bin_id = ? ORDER BY inserted ASC LIMIT 1`, binID).
		Scan(&req.Method, &req.Path, &headersStr, &queryStr, &bodyStr, &req.IP, &req.BinID,
			&req.ReqID, &req.Inserted)

	if err == sql.ErrNoRows {
		http.Error(w, `{"msg":"No requests in this bin"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"msg":"Internal Server Error"}`, http.StatusInternalServerError)
		return
	}

	// Delete the request we just retrieved
	_, err = db.Exec("DELETE FROM requests WHERE req_id = ?", req.ReqID)
	if err != nil {
		http.Error(w, `{"msg":"Internal Server Error"}`, http.StatusInternalServerError)
		return
	}

	json.Unmarshal([]byte(headersStr), &req.Headers)
	json.Unmarshal([]byte(queryStr), &req.Query)
	json.Unmarshal([]byte(bodyStr), &req.Body)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(req)
}

func main() {
	// API routes
	http.HandleFunc("/api/bin", createBinHandler)
	http.HandleFunc("/api/bin/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/bin/" {
			http.NotFound(w, r)
			return
		}

		if r.Method == http.MethodDelete {
			deleteBinHandler(w, r)
		} else if r.Method == http.MethodGet {
			if len(r.URL.Path) > len("/api/bin/")+8 {
				if r.URL.Path[len(r.URL.Path)-len("/req/shift"):] == "/req/shift" {
					shiftRequestHandler(w, r)
				} else if r.URL.Path[len("/api/bin/")+8:len("/api/bin/")+8+len("/req/")] == "/req/" {
					getRequestHandler(w, r)
				} else {
					http.NotFound(w, r)
				}
			} else {
				getBinHandler(w, r)
			}
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Capture all other requests
	http.HandleFunc("/", captureRequestHandler)

	log.Println("Server starting on :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
