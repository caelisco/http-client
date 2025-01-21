package options

import (
	"io"
	"sync"
	"sync/atomic"
)

// progressWriter wraps an io.WriteCloser to track the progress of data being written.
// It reports progress using the onProgress callback, which is invoked with
// the number of bytes written so far and the total bytes expected to be written.
type progressWriter struct {
	writer     io.WriteCloser             // Underlying writer to write data to
	total      int64                      // Total size of the data (in bytes) to be written
	written    atomic.Int64               // Tracks the number of bytes written so far
	onProgress func(written, total int64) // Callback function for reporting progress
	mu         sync.RWMutex
}

// ProgressWriter creates a new progressWriter to monitor writing progress.
// The total parameter specifies the expected total size of the data.
// The onProgress callback is invoked with the current and total written values.
func ProgressWriter(writer io.WriteCloser, total int64, onProgress func(written, total int64)) *progressWriter {
	return &progressWriter{
		writer:     writer,
		total:      total,
		onProgress: onProgress,
	}
}

// Write writes data to the underlying io.WriteCloser and tracks the number of bytes written.
// It invokes the onProgress callback with the updated progress after each write operation.
func (pw *progressWriter) Write(p []byte) (int, error) {
	pw.mu.Lock()
	n, err := pw.writer.Write(p) // Write data to the underlying writer
	pw.mu.Unlock()
	if n > 0 {
		newWritten := pw.written.Add(int64(n)) // Update the total bytes written
		if pw.onProgress != nil {
			pw.onProgress(newWritten, pw.total) // Report progress
		}
	}
	return n, err
}

// Close closes the underlying io.WriteCloser if it implements the io.Closer interface.
// It ensures proper resource cleanup.
func (pw *progressWriter) Close() error {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	if closer, ok := pw.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
