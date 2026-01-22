package httpserver

import (
	"net/http"

	json "github.com/goccy/go-json"
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
	_ = json.NewEncoder(w).Encode(response)
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
