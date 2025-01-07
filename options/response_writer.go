package options

import "io"

// ResponseWriterType defines how the HTTP response body should be handled.
// It determines whether responses are written to an in-memory buffer or directly to a file.
type ResponseWriterType string

// Supported response writer types
const (
	// WriteToBuffer indicates that responses should be written to an in-memory buffer.
	// This is useful for smaller responses that need to be processed in memory.
	WriteToBuffer ResponseWriterType = "buffer"

	// WriteToFile indicates that responses should be written directly to a file.
	// This is recommended for large responses to minimize memory usage.
	WriteToFile ResponseWriterType = "file"
)

// ResponseWriter contains configuration for handling HTTP response bodies.
// It supports writing responses either to an in-memory buffer or directly to a file,
// allowing for flexible response handling based on the needs of the caller.
type ResponseWriter struct {
	// Type determines the destination for response data.
	// Must be either WriteToBuffer or WriteToFile.
	Type ResponseWriterType

	// FilePath specifies the destination file path when Type is WriteToFile.
	// This field is ignored when Type is WriteToBuffer.
	// The path must be writable and will be created if it doesn't exist.
	FilePath string

	// writer is the underlying io.WriteCloser that handles the actual writing.
	// It is initialized during Option.InitialiseWriter() based on the Type.
	// For WriteToBuffer, this will be a bytes.Buffer.
	// For WriteToFile, this will be an *os.File.
	writer io.WriteCloser
}
