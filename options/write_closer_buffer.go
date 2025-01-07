package options

import "bytes"

type WriteCloserBuffer struct {
	*bytes.Buffer
}

func (wcb *WriteCloserBuffer) Close() error {
	// No actual resource to close; just satisfies io.WriteCloser.
	return nil
}
