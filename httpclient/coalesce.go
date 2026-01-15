package httpclient

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"
)

// GenerateCoalesceKey creates a unique key for request deduplication.
// Key = SHA256(method + URL + sorted query params + body hash)
func GenerateCoalesceKey(method, rawURL string, body []byte) string {
	// Parse URL to normalize and sort query params
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		// Fallback to raw URL if parsing fails
		return hashString(method + rawURL + string(body))
	}

	// Sort query parameters for consistent key generation
	queryParams := parsedURL.Query()
	var sortedParams []string
	for key := range queryParams {
		values := queryParams[key]
		sort.Strings(values)
		for _, v := range values {
			sortedParams = append(sortedParams, key+"="+v)
		}
	}
	sort.Strings(sortedParams)

	// Build normalized URL without query (we'll add sorted params)
	normalizedURL := fmt.Sprintf("%s://%s%s", parsedURL.Scheme, parsedURL.Host, parsedURL.Path)

	// Create key components
	keyParts := []string{
		method,
		normalizedURL,
		strings.Join(sortedParams, "&"),
	}

	// Add body hash if present
	if len(body) > 0 {
		bodyHash := sha256.Sum256(body)
		keyParts = append(keyParts, hex.EncodeToString(bodyHash[:]))
	}

	return hashString(strings.Join(keyParts, "|"))
}

// hashString creates a SHA256 hash of the input string.
func hashString(s string) string {
	hash := sha256.Sum256([]byte(s))
	return hex.EncodeToString(hash[:])
}

// perClientCoalesceGroup holds a singleflight group per client.
// This ensures coalescing is scoped to individual clients.
type perClientCoalesceGroup struct {
	mu     sync.RWMutex
	groups map[string]*singleflight.Group
}

var clientCoalesceGroups = &perClientCoalesceGroup{
	groups: make(map[string]*singleflight.Group),
}

// getOrCreateGroup returns the singleflight group for a client.
func (p *perClientCoalesceGroup) getOrCreateGroup(clientID string) *singleflight.Group {
	p.mu.RLock()
	if g, ok := p.groups[clientID]; ok {
		p.mu.RUnlock()
		return g
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if g, ok := p.groups[clientID]; ok {
		return g
	}

	g := &singleflight.Group{}
	p.groups[clientID] = g
	return g
}
