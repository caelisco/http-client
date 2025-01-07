package options

import (
	"io"
	"sync/atomic"
)

// progressReader wraps an io.Reader to track reading progress
type progressReader struct {
	reader     io.Reader
	total      int64
	read       atomic.Int64
	onProgress func(read, total int64)
}

func ProgressReader(reader io.Reader, total int64, onProgress func(read, total int64)) *progressReader {
	return &progressReader{
		reader:     reader,
		total:      total,
		onProgress: onProgress,
	}
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		newRead := pr.read.Add(int64(n))
		if pr.onProgress != nil {
			pr.onProgress(newRead, pr.total)
		}
	}
	return n, err
}

// progressWriter wraps an io.WriteCloser to track writing progress
type progressWriter struct {
	writer     io.WriteCloser // Changed from io.Writer to io.WriteCloser
	total      int64
	written    atomic.Int64
	onProgress func(written, total int64)
}

// ProgressWriter creates a new ProgressWriter
func ProgressWriter(writer io.WriteCloser, total int64, onProgress func(written, total int64)) *progressWriter {
	return &progressWriter{
		writer:     writer,
		total:      total,
		onProgress: onProgress,
	}
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if n > 0 {
		newWritten := pw.written.Add(int64(n))
		if pw.onProgress != nil {
			pw.onProgress(newWritten, pw.total)
		}
	}
	return n, err
}

// Close implements io.Closer interface
func (pw *progressWriter) Close() error {
	if closer, ok := pw.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
