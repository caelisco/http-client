package options

import "io"

// readerCloser wraps an io.Reader and adds a Close() method
type readerCloser struct {
	io.Reader
}

func (r *readerCloser) Close() error {
	return nil
}

// newReaderCloser wraps an io.Reader with a no-op Close method
func newReaderCloser(r io.Reader) io.ReadCloser {
	return &readerCloser{r}
}
