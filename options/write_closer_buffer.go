package options

import "bytes"

// WriteCloserBuffer is a wrapper around bytes.Buffer that implements the io.WriteCloser interface.
// It is used as an in-memory buffer for writing data where closing the buffer does not require
// any additional cleanup or resource management.
type WriteCloserBuffer struct {
	*bytes.Buffer
}

// Close satisfies the io.WriteCloser interface but performs no action.
// This is because there are no resources to release or clean up for an in-memory buffer.
func (wcb *WriteCloserBuffer) Close() error {
	// No actual resource to close; just satisfies io.WriteCloser.
	return nil
}

// Determine if the bytes.Buffer is empty
func (w *WriteCloserBuffer) IsEmpty() bool {
	return w == nil || w.Buffer == nil || w.Buffer.Len() == 0
}
