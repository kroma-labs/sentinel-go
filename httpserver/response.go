package httpserver

import (
	"net/http"

	json "github.com/goccy/go-json"
	"github.com/rs/zerolog/log"
)

// Response is a generic wrapper for all API responses.
//
// This provides a consistent structure across all endpoints:
//   - Data: The actual response payload (type-safe via generics)
//   - Errors: List of field-level errors (for validation failures)
//   - Message: Human-readable message
//
// Example success response:
//
//	{
//	  "data": {"id": 123, "name": "John"},
//	  "message": "user created"
//	}
//
// Example error response:
//
//	{
//	  "errors": [{"field": "email", "message": "invalid format"}],
//	  "message": "validation failed"
//	}
type Response[T any] struct {
	Data    T       `json:"data,omitempty"`
	Errors  []Error `json:"errors,omitempty"`
	Message string  `json:"message,omitempty"`
}

// Error represents a single field-level error.
type Error struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// WriteJSON writes a JSON response with the given status code.
//
// If JSON encoding fails, the error is logged but not returned since
// HTTP headers have already been written at that point.
//
// Example:
//
//	type UserData struct {
//	    ID   int    `json:"id"`
//	    Name string `json:"name"`
//	}
//
//	httpserver.WriteJSON(w, http.StatusOK, Response[UserData]{
//	    Data:    UserData{ID: 1, Name: "John"},
//	    Message: "success",
//	})
func WriteJSON[T any](w http.ResponseWriter, statusCode int, response Response[T]) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Log encoding error - we can't send a new HTTP error since headers are already written
		log.Error().
			Err(err).
			Int("status_code", statusCode).
			Msg("failed to encode JSON response")
	}
}

// WriteError writes a JSON error response.
//
// Example:
//
//	httpserver.WriteError(w, http.StatusBadRequest,
//	    "validation failed",
//	    httpserver.Error{Field: "email", Message: "invalid format"},
//	)
func WriteError(w http.ResponseWriter, statusCode int, message string, errors ...Error) {
	WriteJSON(w, statusCode, Response[any]{
		Errors:  errors,
		Message: message,
	})
}

// WriteSuccess writes a success JSON response with data.
//
// Example:
//
//	httpserver.WriteSuccess(w, http.StatusOK, userData, "user retrieved")
func WriteSuccess[T any](w http.ResponseWriter, statusCode int, data T, message string) {
	WriteJSON(w, statusCode, Response[T]{
		Data:    data,
		Message: message,
	})
}
