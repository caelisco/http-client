package options

import "io"

type ResponseWriterType string

const (
	WriteToBuffer ResponseWriterType = "buffer"
	WriteToFile   ResponseWriterType = "file"
)

// ResponseWriter contains configuration for how to handle the HTTP response body
type ResponseWriter struct {
	// Type indicates whether to write to buffer or file
	Type ResponseWriterType
	// FilePath is only used when Type is WriteToFile
	FilePath string
	// Writer is the underlying io.WriteCloser, initialized based on Type
	writer io.WriteCloser
}
