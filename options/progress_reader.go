package options

import (
	"io"
	"sync"
	"sync/atomic"
)

// progressReader wraps an io.Reader to track the progress of data being read.
// It reports progress using the onProgress callback, which is invoked with
// the number of bytes read so far and the total bytes expected to be read.
type progressReader struct {
	reader     io.Reader               // Underlying reader to read data from
	total      int64                   // Total size of the data (in bytes) to be read
	read       atomic.Int64            // Tracks the number of bytes read so far
	onProgress func(read, total int64) // Callback function for reporting progress
	mu         sync.RWMutex
}

// ProgressReader creates a new progressReader to monitor reading progress.
// The total parameter specifies the expected total size of the data.
// The onProgress callback is invoked with the current and total read values.
func ProgressReader(reader io.Reader, total int64, onProgress func(read, total int64)) *progressReader {
	return &progressReader{
		reader:     reader,
		total:      total,
		onProgress: onProgress,
	}
}

// Read reads data from the underlying io.Reader and tracks the number of bytes read.
// It invokes the onProgress callback with the updated progress after each read operation.
func (pr *progressReader) Read(p []byte) (int, error) {
	pr.mu.RLock()
	n, err := pr.reader.Read(p) // Read data from the underlying reader
	pr.mu.RUnlock()
	if n > 0 {
		newRead := pr.read.Add(int64(n)) // Update the total bytes read
		if pr.onProgress != nil {
			pr.onProgress(newRead, pr.total) // Report progress
		}
	}
	return n, err
}
