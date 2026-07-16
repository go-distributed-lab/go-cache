package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-cache/internal/eviction"
	"go-cache/internal/logger"
	"go-cache/pkg/cache"
	"io"
)

// newTestServer builds a test HTTP server backed by the given cache.
func newTestServer(t *testing.T, c cache.Cache[string, string]) *httptest.Server {
	t.Helper()
	log := logger.New(io.Discard)

	mux := http.NewServeMux()

	getHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}

	metricsHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		stats := c.Metrics()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"hits":      stats.Hits,
			"misses":    stats.Misses,
			"evictions": stats.Evictions,
			"hit_rate":  stats.HitRate(),
			"len":       c.Len(),
		})
	}

	cacheHandler := func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path[len("/cache/"):]
		if key == "" {
			http.Error(w, "key required", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodGet:
			value, ok := c.Get(key)
			if !ok {
				http.Error(w, "cache miss", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"key": key, "value": value})

		case http.MethodPost:
			var body struct {
				Value string `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Value == "" {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			c.Set(key, body.Value)
			log.Info("set", "key", key)
			w.WriteHeader(http.StatusCreated)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"key": key, "value": body.Value})

		case http.MethodDelete:
			c.Delete(key)
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}

	mux.HandleFunc("/health", getHandler)
	mux.HandleFunc("/metrics", metricsHandler)
	mux.HandleFunc("/cache/", cacheHandler)

	return httptest.NewServer(mux)
}

func TestAPI_Health(t *testing.T) {
	srv := newTestServer(t, eviction.NewLRU[string, string](16))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestAPI_SetAndGet(t *testing.T) {
	srv := newTestServer(t, eviction.NewLRU[string, string](16))
	defer srv.Close()

	// Set
	body := bytes.NewBufferString(`{"value":"golang"}`)
	resp, err := http.Post(srv.URL+"/cache/lang", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("want 201, got %d", resp.StatusCode)
	}

	// Get
	resp, err = http.Get(srv.URL + "/cache/lang")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["value"] != "golang" {
		t.Errorf("want value=golang, got %q", result["value"])
	}
}

func TestAPI_GetMiss(t *testing.T) {
	srv := newTestServer(t, eviction.NewLRU[string, string](16))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/cache/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestAPI_Delete(t *testing.T) {
	srv := newTestServer(t, eviction.NewLRU[string, string](16))
	defer srv.Close()

	// Set then delete
	body := bytes.NewBufferString(`{"value":"todelete"}`)
	resp, _ := http.Post(srv.URL+"/cache/temp", "application/json", body)
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/cache/temp", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("want 204, got %d", resp.StatusCode)
	}

	// Confirm gone
	resp, _ = http.Get(srv.URL + "/cache/temp")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404 after delete, got %d", resp.StatusCode)
	}
}

func TestAPI_Metrics(t *testing.T) {
	srv := newTestServer(t, eviction.NewLRU[string, string](16))
	defer srv.Close()

	// Generate a hit and a miss
	body := bytes.NewBufferString(`{"value":"v"}`)
	resp, _ := http.Post(srv.URL+"/cache/k", "application/json", body)
	resp.Body.Close()
	resp, _ = http.Get(srv.URL + "/cache/k")
	resp.Body.Close()
	resp, _ = http.Get(srv.URL + "/cache/missing")
	resp.Body.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)

	if m["hits"].(float64) != 1 {
		t.Errorf("want hits=1, got %v", m["hits"])
	}
	if m["misses"].(float64) != 1 {
		t.Errorf("want misses=1, got %v", m["misses"])
	}
}

func TestAPI_TTLStrategy(t *testing.T) {
	srv := newTestServer(t, eviction.NewTTL[string, string](50*time.Millisecond))
	defer srv.Close()

	body := bytes.NewBufferString(`{"value":"expires"}`)
	resp, _ := http.Post(srv.URL+"/cache/x", "application/json", body)
	resp.Body.Close()

	time.Sleep(100 * time.Millisecond)

	resp, _ = http.Get(srv.URL + "/cache/x")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404 after TTL expiry, got %d", resp.StatusCode)
	}
}

func TestAPI_InvalidBody(t *testing.T) {
	srv := newTestServer(t, eviction.NewLRU[string, string](16))
	defer srv.Close()

	body := bytes.NewBufferString(`not json`)
	resp, err := http.Post(srv.URL+"/cache/k", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400, got %d", resp.StatusCode)
	}
}

func TestAPI_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t, eviction.NewLRU[string, string](16))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/cache/k", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", resp.StatusCode)
	}
}
