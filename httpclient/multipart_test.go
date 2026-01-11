package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestBuilder_File(t *testing.T) {
	// Create a temp file for testing
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test file content"), 0o644)
	require.NoError(t, err)

	client := New()
	rb := client.Request("upload").File("document", testFile)

	assert.Len(t, rb.fileUploads, 1)
	assert.Equal(t, "document", rb.fileUploads[0].FieldName)
	assert.Equal(t, "test.txt", rb.fileUploads[0].FileName)
}

func TestRequestBuilder_FileReader(t *testing.T) {
	content := "test file content from reader"
	reader := strings.NewReader(content)

	client := New()
	rb := client.Request("upload").FileReader("image", "photo.jpg", reader)

	assert.Len(t, rb.fileUploads, 1)
	assert.Equal(t, "image", rb.fileUploads[0].FieldName)
	assert.Equal(t, "photo.jpg", rb.fileUploads[0].FileName)
	assert.Equal(t, reader, rb.fileUploads[0].Reader)
}

func TestRequestBuilder_FormField(t *testing.T) {
	client := New()
	rb := client.Request("upload").
		FormField("title", "My Document").
		FormField("category", "reports")

	assert.Equal(t, "My Document", rb.formFields["title"])
	assert.Equal(t, "reports", rb.formFields["category"])
}

func TestRequestBuilder_MultipleFiles(t *testing.T) {
	client := New()
	rb := client.Request("upload").
		FileReader("file1", "doc1.pdf", strings.NewReader("content1")).
		FileReader("file2", "doc2.pdf", strings.NewReader("content2")).
		FormField("description", "Multiple files")

	assert.Len(t, rb.fileUploads, 2)
	assert.Len(t, rb.formFields, 1)
}

func TestRequestBuilder_MultipartUpload(t *testing.T) {
	var receivedContentType string
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(WithBaseURL(server.URL))

	resp, err := client.Request("upload").
		FileReader("document", "test.txt", strings.NewReader("file content")).
		FormField("title", "Test Upload").
		Post(context.Background(), "/upload")

	require.NoError(t, err)
	assert.True(t, resp.IsSuccess())
	assert.Contains(t, receivedContentType, "multipart/form-data")
	assert.Contains(t, string(receivedBody), "file content")
	assert.Contains(t, string(receivedBody), "Test Upload")
}

func TestBuildMultipart(t *testing.T) {
	client := New()
	rb := client.Request("upload").
		FileReader("doc", "test.txt", strings.NewReader("hello world")).
		FormField("name", "test")

	body, contentType, err := rb.buildMultipart()

	require.NoError(t, err)
	assert.Contains(t, contentType, "multipart/form-data")
	assert.Contains(t, body.String(), "hello world")
	assert.Contains(t, body.String(), "name")
	assert.Contains(t, body.String(), "test")
}

func TestBuildMultipart_WithRealFile(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "upload.txt")
	err := os.WriteFile(testFile, []byte("real file content"), 0o644)
	require.NoError(t, err)

	client := New()
	rb := client.Request("upload").
		File("document", testFile).
		FormField("title", "Real File")

	body, contentType, err := rb.buildMultipart()

	require.NoError(t, err)
	assert.Contains(t, contentType, "multipart/form-data")
	assert.Contains(t, body.String(), "real file content")
}

func TestBuildMultipart_FileNotFound(t *testing.T) {
	client := New()
	rb := client.Request("upload").
		File("document", "/nonexistent/file.txt")

	_, _, err := rb.buildMultipart()

	assert.Error(t, err)
}

func TestLazyFileReader_Read(t *testing.T) {
	lazy := &lazyFileReader{path: "/some/path"}
	buf := make([]byte, 10)
	n, err := lazy.Read(buf)

	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}
