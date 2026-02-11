package httpx

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// WriteJSON writes a JSON payload with the provided status code.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// Error writes a standard error envelope.
func Error(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]any{
		"error": message,
	})
}

// ReadJSON decodes the request body as JSON into dst.
func ReadJSON(r *http.Request, dst any) error {
	defer func() {
		_ = r.Body.Close()
	}()
	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(dst)
}

// QueryInt fetches an integer query parameter with a default value.
func QueryInt(r *http.Request, key string, defaultValue int) int {
	val := r.URL.Query().Get(key)
	if val == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return defaultValue
	}
	return parsed
}

// QueryString fetches a query string parameter with a default value.
func QueryString(r *http.Request, key, defaultValue string) string {
	val := r.URL.Query().Get(key)
	if val == "" {
		return defaultValue
	}
	return val
}

// ClientIP attempts to determine the real client IP respecting proxy headers.
func ClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
