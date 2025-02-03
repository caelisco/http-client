package options

import (
	"io"
)

// progressWriter wraps an io.WriteCloser with progress tracking capabilities.
// It coordinates writing data while monitoring bytes written.
type progressWriter struct {
	writer io.WriteCloser
	prog   *progress
}

// NewProgressWriter returns an io.Writer that tracks progress during write operations.
// The onProgress callback is invoked with current bytes written and total size.
// Use for scenarios like download progress tracking.
func NewProgressWriter(w io.Writer, totalSize int64, onProgress func(current, total int64)) io.Writer {
	p := &progress{
		totalSize:  totalSize,
		onProgress: onProgress,
	}
	return io.MultiWriter(w, p)
}

// Write implements io.Writer and updates progress tracking.
// Returns number of bytes written and any error encountered.
func (w *progressWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 {
		w.prog.Write(p[:n])
	}
	return n, err
}

// Close implements io.Closer, closing the underlying writer if it implements io.Closer.
// Returns any error from closing.
func (w *progressWriter) Close() error {
	if closer, ok := w.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
