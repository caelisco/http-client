package options

import (
	"io"
	"sync"
	"sync/atomic"
)

// progressReader wraps an io.Reader to track the progress of data being read.
// It reports progress using the onProgress callback, which is invoked with
// the number of bytes read so far and the total bytes expected to be read.
type ProgressReader struct {
	reader     io.Reader
	total      int64
	read       atomic.Int64
	onProgress func(read, total int64)

	// Protects callback execution and any shared state
	callbackMu sync.Mutex

	// Track if this reader is part of a redirect chain
	isRedirect bool
	parent     *ProgressReader // Reference to original reader in redirect chain
}

// ProgressReader creates a new progressReader to monitor reading progress.
// The total parameter specifies the expected total size of the data.
// The onProgress callback is invoked with the current and total read values.
func NewProgressReader(reader io.Reader, total int64, onProgress func(read, total int64)) *ProgressReader {
	return &ProgressReader{
		reader:     reader,
		total:      total,
		onProgress: onProgress,
	}
}

// CloneForRedirect creates a new progressReader that shares progress tracking
// with the original reader, making it safe for redirect chains
func (pr *ProgressReader) CloneForRedirect() *ProgressReader {
	if pr.isRedirect {
		// If this is already a redirect clone, use its parent
		return &ProgressReader{
			parent:     pr.parent,
			isRedirect: true,
			total:      pr.total,
			onProgress: pr.parent.onProgress,
		}
	}

	return &ProgressReader{
		parent:     pr,
		isRedirect: true,
		total:      pr.total,
		onProgress: pr.onProgress,
	}
}

// Read reads data from the underlying io.Reader and tracks the number of bytes read.
func (pr *ProgressReader) Read(p []byte) (int, error) {
	// If this is a redirect clone, delegate to parent
	if pr.isRedirect && pr.parent != nil {
		return pr.parent.Read(p)
	}

	n, err := pr.reader.Read(p)

	if n > 0 {
		newRead := pr.read.Add(int64(n))
		if pr.onProgress != nil {
			pr.callbackMu.Lock()
			pr.onProgress(newRead, pr.total)
			pr.callbackMu.Unlock()
		}
	}
	return n, err
}

// GetProgress returns the current number of bytes read
func (pr *ProgressReader) GetProgress() int64 {
	if pr.isRedirect && pr.parent != nil {
		return pr.parent.GetProgress()
	}
	return pr.read.Load()
}
