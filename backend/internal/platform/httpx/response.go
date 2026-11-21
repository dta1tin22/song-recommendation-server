// Package httpx holds the HTTP response helpers shared by every layer that can
// write to the wire: handlers, middleware and the router-level fallbacks in
// cmd/api. It exists so the error envelope is defined exactly once, and so
// middleware can emit a 401 without importing the handler package (which imports
// middleware for request context, and would therefore be a cycle).
package httpx

import (
	"encoding/json"
	"log"
	"net/http"
)

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteJSON serializes payload with the given status code. A nil payload writes
// only the status (used by the 204 responses).
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("httpx: encode response: %v", err)
	}
}

// WriteError emits the shared error envelope:
// {"error":{"code":"...","message":"..."}}
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}
