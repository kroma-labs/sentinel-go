package httpclient

import (
	"bytes"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
)

// FileUpload represents a file to be uploaded in a multipart request.
//
// Use File() or FileReader() on RequestBuilder to add file uploads.
//
// Example - Upload from path:
//
//	resp, err := client.Request("Upload").
//	    File("document", "/path/to/file.pdf").
//	    Post(ctx, "/upload")
//
// Example - Upload from reader:
//
//	resp, err := client.Request("Upload").
//	    FileReader("image", "photo.jpg", imageData).
//	    Post(ctx, "/upload")
type FileUpload struct {
	// FieldName is the form field name for the file.
	// This is the name used in the multipart form data.
	//
	// Example: "document", "avatar", "attachment"
	FieldName string

	// FileName is the name of the file as it appears in the upload.
	// This is typically the original filename or a custom name.
	//
	// Example: "report.pdf", "profile.jpg"
	FileName string

	// Reader provides the file content.
	// For file paths, this is automatically created from os.Open.
	// For in-memory data, use bytes.NewReader or strings.NewReader.
	Reader io.Reader
}

// File adds a file upload from a file path.
//
// The file is opened when the request is executed. If the file doesn't exist
// or cannot be read, an error is returned during execution.
//
// Parameters:
//   - fieldName: the form field name (e.g., "document", "avatar")
//   - filePath: absolute or relative path to the file
//
// Example:
//
//	resp, err := client.Request("UploadDoc").
//	    File("document", "/path/to/report.pdf").
//	    FormField("title", "Q4 Report").
//	    Post(ctx, "/upload")
func (rb *RequestBuilder) File(fieldName, filePath string) *RequestBuilder {
	if rb.fileUploads == nil {
		rb.fileUploads = make([]FileUpload, 0)
	}

	rb.fileUploads = append(rb.fileUploads, FileUpload{
		FieldName: fieldName,
		FileName:  filepath.Base(filePath),
		Reader:    &lazyFileReader{path: filePath},
	})

	return rb
}

// FileReader adds a file upload from an io.Reader.
//
// Use this for in-memory data, streams, or when you already have the file
// content loaded.
//
// Parameters:
//   - fieldName: the form field name (e.g., "document", "avatar")
//   - fileName: the filename to use in the upload
//   - reader: the file content source
//
// Example:
//
//	// Upload from bytes
//	resp, err := client.Request("UploadImage").
//	    FileReader("avatar", "profile.png", bytes.NewReader(imageBytes)).
//	    Post(ctx, "/avatar")
//
//	// Upload from string
//	resp, err := client.Request("UploadCSV").
//	    FileReader("data", "export.csv", strings.NewReader(csvData)).
//	    Post(ctx, "/import")
func (rb *RequestBuilder) FileReader(fieldName, fileName string, reader io.Reader) *RequestBuilder {
	if rb.fileUploads == nil {
		rb.fileUploads = make([]FileUpload, 0)
	}

	rb.fileUploads = append(rb.fileUploads, FileUpload{
		FieldName: fieldName,
		FileName:  fileName,
		Reader:    reader,
	})

	return rb
}

// FormField adds a form field to a multipart request.
//
// This is used together with File() or FileReader() to add additional
// form fields to the multipart request.
//
// Example:
//
//	resp, err := client.Request("Upload").
//	    File("document", "/path/to/file.pdf").
//	    FormField("title", "My Document").
//	    FormField("category", "reports").
//	    Post(ctx, "/upload")
func (rb *RequestBuilder) FormField(key, value string) *RequestBuilder {
	if rb.formFields == nil {
		rb.formFields = make(map[string]string)
	}
	rb.formFields[key] = value
	return rb
}

// buildMultipart creates a multipart form body from files and fields.
func (rb *RequestBuilder) buildMultipart() (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add form fields
	for key, value := range rb.formFields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", err
		}
	}

	// Add files
	for _, file := range rb.fileUploads {
		// Resolve lazy file readers
		reader := file.Reader
		if lazy, ok := reader.(*lazyFileReader); ok {
			f, err := os.Open(lazy.path)
			if err != nil {
				return nil, "", err
			}
			defer f.Close()
			reader = f
		}

		part, err := writer.CreateFormFile(file.FieldName, file.FileName)
		if err != nil {
			return nil, "", err
		}

		if _, err := io.Copy(part, reader); err != nil {
			return nil, "", err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}

	return body, writer.FormDataContentType(), nil
}

// lazyFileReader defers file opening until the request is executed.
type lazyFileReader struct {
	path string
}

func (l *lazyFileReader) Read(_ []byte) (int, error) {
	// This should never be called directly - buildMultipart handles it
	return 0, io.EOF
}
