package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	// Use in-memory SQLite for testing
	var err error
	testDB, err = sql.Open("sqlite3", ":memory:")
	if err != nil {
		panic(err)
	}

	// Set the global db variable to our test database
	db = testDB

	// Create tables
	_, err = testDB.Exec(`
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
		panic(err)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	testDB.Close()
	os.Exit(code)
}

// Helper function to clear the database between tests
func clearDB(t *testing.T) {
	_, err := testDB.Exec("DELETE FROM requests")
	if err != nil {
		t.Fatalf("Failed to clear requests table: %v", err)
	}
	_, err = testDB.Exec("DELETE FROM bins")
	if err != nil {
		t.Fatalf("Failed to clear bins table: %v", err)
	}
}

func TestCreateBin(t *testing.T) {
	clearDB(t)

	req := httptest.NewRequest(http.MethodPost, "/api/bin", nil)
	w := httptest.NewRecorder()

	createBinHandler(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status code %d, got %d", http.StatusCreated, w.Code)
	}

	var bin Bin
	if err := json.NewDecoder(w.Body).Decode(&bin); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(bin.BinID) != 8 {
		t.Errorf("Expected binID length 8, got %d", len(bin.BinID))
	}

	if bin.Expires <= bin.Now {
		t.Error("Expiration time should be in the future")
	}

	if bin.Entries != 0 {
		t.Errorf("Expected 0 entries for new bin, got %d", bin.Entries)
	}
}

func TestGetBin(t *testing.T) {
	clearDB(t)

	// First create a bin
	createReq := httptest.NewRequest(http.MethodPost, "/api/bin", nil)
	createW := httptest.NewRecorder()
	createBinHandler(createW, createReq)

	var bin Bin
	json.NewDecoder(createW.Body).Decode(&bin)

	// Then try to get it
	req := httptest.NewRequest(http.MethodGet, "/api/bin/"+bin.BinID, nil)
	w := httptest.NewRecorder()
	getBinHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	var retrievedBin Bin
	if err := json.NewDecoder(w.Body).Decode(&retrievedBin); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if retrievedBin.BinID != bin.BinID {
		t.Errorf("Expected binID %s, got %s", bin.BinID, retrievedBin.BinID)
	}
}

func TestCaptureRequest(t *testing.T) {
	clearDB(t)

	// Create a bin first
	createReq := httptest.NewRequest(http.MethodPost, "/api/bin", nil)
	createW := httptest.NewRecorder()
	createBinHandler(createW, createReq)

	var bin Bin
	json.NewDecoder(createW.Body).Decode(&bin)

	// Send a request to capture
	body := []byte(`{"test":"data"}`)
	req := httptest.NewRequest(http.MethodPost, "/"+bin.BinID+"?key=value", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	captureRequestHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	reqID := w.Body.String()
	if len(reqID) != 8 {
		t.Errorf("Expected reqID length 8, got %d", len(reqID))
	}
}

func TestShiftRequest(t *testing.T) {
	clearDB(t)

	// Create a bin
	createReq := httptest.NewRequest(http.MethodPost, "/api/bin", nil)
	createW := httptest.NewRecorder()
	createBinHandler(createW, createReq)

	var bin Bin
	json.NewDecoder(createW.Body).Decode(&bin)

	// Capture a request
	body := []byte(`{"test":"data"}`)
	captureReq := httptest.NewRequest(http.MethodPost, "/"+bin.BinID, bytes.NewBuffer(body))
	captureW := httptest.NewRecorder()
	captureRequestHandler(captureW, captureReq)

	// Try to shift the request
	shiftReq := httptest.NewRequest(http.MethodGet, "/api/bin/"+bin.BinID+"/req/shift", nil)
	shiftW := httptest.NewRecorder()
	shiftRequestHandler(shiftW, shiftReq)

	if shiftW.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, shiftW.Code)
	}

	var req Request
	if err := json.NewDecoder(shiftW.Body).Decode(&req); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if req.BinID != bin.BinID {
		t.Errorf("Expected binID %s, got %s", bin.BinID, req.BinID)
	}

	if req.Method != http.MethodPost {
		t.Errorf("Expected method POST, got %s", req.Method)
	}

	// Try to shift again - should be empty
	shiftW = httptest.NewRecorder()
	shiftRequestHandler(shiftW, shiftReq)

	if shiftW.Code != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, shiftW.Code)
	}
}

func TestDeleteBin(t *testing.T) {
	clearDB(t)

	// Create a bin
	createReq := httptest.NewRequest(http.MethodPost, "/api/bin", nil)
	createW := httptest.NewRecorder()
	createBinHandler(createW, createReq)

	var bin Bin
	json.NewDecoder(createW.Body).Decode(&bin)

	// Delete the bin
	req := httptest.NewRequest(http.MethodDelete, "/api/bin/"+bin.BinID, nil)
	w := httptest.NewRecorder()
	deleteBinHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// Try to get the deleted bin
	getReq := httptest.NewRequest(http.MethodGet, "/api/bin/"+bin.BinID, nil)
	getW := httptest.NewRecorder()
	getBinHandler(getW, getReq)

	if getW.Code != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, getW.Code)
	}
}

func TestExpiredBin(t *testing.T) {
	clearDB(t)

	// Create a bin that's already expired
	binID := generateID()
	now := time.Now().UnixMilli()
	expires := now - 1000 // expired 1 second ago

	_, err := testDB.Exec("INSERT INTO bins (bin_id, created_at, expires_at) VALUES (?, ?, ?)",
		binID, now, expires)
	if err != nil {
		t.Fatalf("Failed to create expired bin: %v", err)
	}

	// Try to capture a request
	req := httptest.NewRequest(http.MethodPost, "/"+binID, strings.NewReader("test"))
	w := httptest.NewRecorder()
	captureRequestHandler(w, req)

	if w.Code != http.StatusGone {
		t.Errorf("Expected status code %d for expired bin, got %d", http.StatusGone, w.Code)
	}
}
