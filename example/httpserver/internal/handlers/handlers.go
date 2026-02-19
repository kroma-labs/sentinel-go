package handlers

import (
	cryrand "crypto/rand"
	"encoding/json"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// User represents a user entity
type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// APIResponse represents a standard API response
type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// HomeHandler handles the home route
func HomeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, APIResponse{
			Success: true,
			Message: "Welcome to Sentinel HTTP Server Example",
			Data: map[string]string{
				"version": "0.1.0",
				"service": "sentinel-httpserver-example",
			},
		})
	}
}

// ListUsersHandler handles GET /api/users
func ListUsersHandler() http.HandlerFunc {
	users := []User{
		{
			ID:        1,
			Name:      "Alice",
			Email:     "alice@example.com",
			CreatedAt: time.Now().Add(-24 * time.Hour),
		},
		{ID: 2, Name: "Bob", Email: "bob@example.com", CreatedAt: time.Now().Add(-48 * time.Hour)},
		{
			ID:        3,
			Name:      "Charlie",
			Email:     "charlie@example.com",
			CreatedAt: time.Now().Add(-72 * time.Hour),
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time
		time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)

		writeJSON(w, http.StatusOK, APIResponse{
			Success: true,
			Data:    users,
		})
	}
}

// GetUserHandler handles GET /api/users/{id}
func GetUserHandler() http.HandlerFunc {
	users := map[int]User{
		1: {
			ID:        1,
			Name:      "Alice",
			Email:     "alice@example.com",
			CreatedAt: time.Now().Add(-24 * time.Hour),
		},
		2: {
			ID:        2,
			Name:      "Bob",
			Email:     "bob@example.com",
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
		3: {
			ID:        3,
			Name:      "Charlie",
			Email:     "charlie@example.com",
			CreatedAt: time.Now().Add(-72 * time.Hour),
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time
		time.Sleep(time.Duration(rand.Intn(30)) * time.Millisecond)

		// Extract user ID from path
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, APIResponse{
				Success: false,
				Error:   "Invalid user ID",
			})
			return
		}

		user, exists := users[id]
		if !exists {
			writeJSON(w, http.StatusNotFound, APIResponse{
				Success: false,
				Error:   "User not found",
			})
			return
		}

		writeJSON(w, http.StatusOK, APIResponse{
			Success: true,
			Data:    user,
		})
	}
}

// CreateUserHandler handles POST /api/users
func CreateUserHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time
		time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)

		var input struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, APIResponse{
				Success: false,
				Error:   "Invalid request body",
			})
			return
		}

		if input.Name == "" || input.Email == "" {
			writeJSON(w, http.StatusBadRequest, APIResponse{
				Success: false,
				Error:   "Name and email are required",
			})
			return
		}

		user := User{
			ID:        rand.Intn(1000) + 100,
			Name:      input.Name,
			Email:     input.Email,
			CreatedAt: time.Now(),
		}

		writeJSON(w, http.StatusCreated, APIResponse{
			Success: true,
			Message: "User created successfully",
			Data:    user,
		})
	}
}

// UpdateUserHandler handles PUT /api/users/{id}
func UpdateUserHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time
		time.Sleep(time.Duration(rand.Intn(80)) * time.Millisecond)

		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, APIResponse{
				Success: false,
				Error:   "Invalid user ID",
			})
			return
		}

		var input struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, APIResponse{
				Success: false,
				Error:   "Invalid request body",
			})
			return
		}

		user := User{
			ID:        id,
			Name:      input.Name,
			Email:     input.Email,
			CreatedAt: time.Now().Add(-24 * time.Hour), // Simulated original creation
		}

		writeJSON(w, http.StatusOK, APIResponse{
			Success: true,
			Message: "User updated successfully",
			Data:    user,
		})
	}
}

// DeleteUserHandler handles DELETE /api/users/{id}
func DeleteUserHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time
		time.Sleep(time.Duration(rand.Intn(40)) * time.Millisecond)

		idStr := r.PathValue("id")
		_, err := strconv.Atoi(idStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, APIResponse{
				Success: false,
				Error:   "Invalid user ID",
			})
			return
		}

		writeJSON(w, http.StatusOK, APIResponse{
			Success: true,
			Message: "User deleted successfully",
		})
	}
}

// SlowHandler simulates a slow endpoint for testing
func SlowHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse delay from query parameter
		delayStr := r.URL.Query().Get("delay")
		delay := 2 * time.Second
		if delayStr != "" {
			if d, err := time.ParseDuration(delayStr); err == nil {
				delay = d
			}
		}

		time.Sleep(delay)

		writeJSON(w, http.StatusOK, APIResponse{
			Success: true,
			Message: "Slow response completed",
			Data: map[string]string{
				"delay": delay.String(),
			},
		})
	}
}

// ErrorHandler simulates an error endpoint for testing
func ErrorHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Random chance of returning different error codes
		codes := []int{400, 401, 403, 404, 500, 502, 503}
		code := codes[rand.Intn(len(codes))]

		writeJSON(w, code, APIResponse{
			Success: false,
			Error:   http.StatusText(code),
		})
	}
}

// RandomHandler returns random data with varying latencies
func RandomHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Random latency between 10-200ms
		latency := time.Duration(rand.Intn(190)+10) * time.Millisecond
		time.Sleep(latency)

		// Random data
		data := map[string]any{
			"random_number": rand.Intn(1000),
			"random_float":  rand.Float64(),
			"timestamp":     time.Now().UnixMilli(),
			"latency_ms":    latency.Milliseconds(),
		}

		writeJSON(w, http.StatusOK, APIResponse{
			Success: true,
			Data:    data,
		})
	}
}

// PatchUserHandler handles PATCH /api/users/{id}
func PatchUserHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time
		time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)

		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, APIResponse{
				Success: false,
				Error:   "Invalid user ID",
			})
			return
		}

		// Use a map to handle partial updates
		var updates map[string]any
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			writeJSON(w, http.StatusBadRequest, APIResponse{
				Success: false,
				Error:   "Invalid request body",
			})
			return
		}

		// Simulate updating specific fields...
		user := User{
			ID:        id,
			Name:      "Patched User", // specific logic omitted for brevity
			Email:     "patched@example.com",
			CreatedAt: time.Now().Add(-24 * time.Hour),
		}
		if name, ok := updates["name"].(string); ok {
			user.Name = name
		}
		if email, ok := updates["email"].(string); ok {
			user.Email = email
		}

		writeJSON(w, http.StatusOK, APIResponse{
			Success: true,
			Message: "User patched successfully",
			Data:    user,
		})
	}
}

// UploadHandler handles POST /api/upload
// Accepts any content and returns success. Used to test large request bodies.
func UploadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Simulate processing a file upload
		time.Sleep(time.Duration(rand.Intn(200)) * time.Millisecond)

		// Just read the body count (metrics middleware handles the actual size tracking)
		// in a real app we'd save the file
		size := r.ContentLength

		writeJSON(w, http.StatusOK, APIResponse{
			Success: true,
			Message: "Upload received",
			Data: map[string]any{
				"size_bytes": size,
			},
		})
	}
}

// DownloadHandler handles GET /api/download
// Returns a random byte array of requested size. Used to test large response bodies.
func DownloadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sizeStr := r.URL.Query().Get("size")
		size := 1024 // default 1KB
		if sizeStr != "" {
			if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 {
				size = s
			}
		}

		// Generate random data
		data := make([]byte, size)
		cryrand.Read(data)

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(size))
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}
}
