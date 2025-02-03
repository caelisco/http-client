package options

import (
	"io"
	"sync/atomic"
)

// progress wraps an io.Writer to track bytes processed during I/O operations.
// It uses atomic operations to safely track progress in concurrent scenarios.
type progress struct {
	current    atomic.Int64
	totalSize  int64
	onProgress func(current, total int64)
}

// NewProgressReader returns an io.Reader that reports progress during read operations.
// If totalSize is <= 0, it attempts to determine size using io.Seeker if available.
// The onProgress callback receives current bytes read and total size (-1 if unknown).
func NewProgressReader(r io.Reader, totalSize int64, onProgress func(current, total int64)) io.Reader {
	if totalSize <= 0 {
		// Try to get size from Seeker if available
		if seeker, ok := r.(io.Seeker); ok {
			if size, err := seeker.Seek(0, io.SeekEnd); err == nil {
				seeker.Seek(0, io.SeekStart) // Reset position
				totalSize = size
			}
		}
	}

	p := &progress{
		totalSize:  totalSize,
		onProgress: onProgress,
	}
	return io.TeeReader(r, p)
}

// Write implements io.Writer and updates progress atomically.
// Returns number of bytes written and any error that occurred.
func (p *progress) Write(b []byte) (int, error) {
	n := len(b)
	current := p.current.Add(int64(n))
	if p.onProgress != nil {
		if p.totalSize > 0 {
			// We know the total size, report percentage
			p.onProgress(current, p.totalSize)
		} else {
			// Unknown total size (e.g. compressed content)
			// Just report the bytes read
			p.onProgress(current, -1)
		}
	}
	return n, nil
}

// Reset zeroes the progress counter back to its initial state.
func (p *progress) Reset() {
	p.current.Store(0)
}
