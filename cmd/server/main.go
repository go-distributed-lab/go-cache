package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"strings"
	"time"

	"go-cache/internal/eviction"
	"go-cache/internal/logger"
	"go-cache/pkg/cache"
)

// server holds the active cache and logger.
type server struct {
	cache  cache.Cache[string, string]
	logger *logger.Logger
}

func main() {
	strategy := flag.String("strategy", "lru", "cache strategy: lru, lfu, fifo, ttl, sharded")
	capacity := flag.Int("capacity", 1024, "max entries (not used for ttl)")
	ttlDur := flag.Duration("ttl", 5*time.Minute, "TTL duration (ttl strategy only)")
	shards := flag.Int("shards", 16, "number of shards (sharded strategy only)")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	log := logger.Default()
	c := buildCache(*strategy, *capacity, *ttlDur, *shards, log)

	s := &server{cache: c, logger: log}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/cache/", s.handleCache)

	log.Info("starting go-cache server",
		"strategy", *strategy,
		"capacity", *capacity,
		"addr", *addr,
	)

	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Error("server stopped", "err", err)
	}
}

func buildCache(strategy string, capacity int, ttlDur time.Duration, shards int, log *logger.Logger) cache.Cache[string, string] {
	switch strings.ToLower(strategy) {
	case "lru":
		log.Info("cache strategy", "type", "LRU", "capacity", capacity)
		return eviction.NewLRU[string, string](capacity)
	case "lfu":
		log.Info("cache strategy", "type", "LFU", "capacity", capacity)
		return eviction.NewLFU[string, string](capacity)
	case "fifo":
		log.Info("cache strategy", "type", "FIFO", "capacity", capacity)
		return eviction.NewFIFO[string, string](capacity)
	case "ttl":
		log.Info("cache strategy", "type", "TTL", "ttl", ttlDur)
		return eviction.NewTTL[string, string](ttlDur)
	case "sharded":
		log.Info("cache strategy", "type", "Sharded", "shards", shards, "capacity", capacity)
		return eviction.NewSharded[string](shards, capacity)
	default:
		log.Info("unknown strategy, falling back to LRU", "strategy", strategy)
		return eviction.NewLRU[string, string](capacity)
	}
}

// handleHealth responds with a simple liveness check.
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleMetrics returns a JSON snapshot of cache counters.
func (s *server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stats := s.cache.Metrics()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"hits":      stats.Hits,
		"misses":    stats.Misses,
		"evictions": stats.Evictions,
		"hit_rate":  stats.HitRate(),
		"len":       s.cache.Len(),
	})
}

// handleCache routes GET / POST / DELETE /cache/{key}.
func (s *server) handleCache(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/cache/")
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGet(w, r, key)
	case http.MethodPost:
		s.handleSet(w, r, key)
	case http.MethodDelete:
		s.handleDelete(w, r, key)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) handleGet(w http.ResponseWriter, _ *http.Request, key string) {
	value, ok := s.cache.Get(key)
	if !ok {
		http.Error(w, "cache miss", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"key": key, "value": value})
}

func (s *server) handleSet(w http.ResponseWriter, r *http.Request, key string) {
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if body.Value == "" {
		http.Error(w, "value required", http.StatusBadRequest)
		return
	}
	s.cache.Set(key, body.Value)
	s.logger.Info("set", "key", key)
	w.Header().Set("Content-Type", "application/json") // must be before WriteHeader
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"key": key, "value": body.Value})
}

func (s *server) handleDelete(w http.ResponseWriter, _ *http.Request, key string) {
	s.cache.Delete(key)
	s.logger.Info("delete", "key", key)
	w.WriteHeader(http.StatusNoContent)
}

// compile-time check: suppress unused import warning for "log"
