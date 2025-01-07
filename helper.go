package client

import (
	"bytes"
	"fmt"
	"io"
	netURL "net/url"
	"strings"

	"github.com/caelisco/http-client/options"
)

func normaliseURL(url string, protocolScheme string) (string, error) {
	url = strings.TrimSpace(url)

	// First validate if the input URL has proper scheme format if it contains a colon
	if strings.Contains(url, ":") {
		if !strings.Contains(url, "://") {
			return "", fmt.Errorf("invalid URL format: missing // after scheme")
		}
	}

	if protocolScheme != "" {
		url = strings.TrimPrefix(url, string(SchemeHTTP))
		url = strings.TrimPrefix(url, string(SchemeHTTPS))
		if !strings.Contains(protocolScheme, "://") {
			protocolScheme += "://"
		}
		if !strings.HasPrefix(url, protocolScheme) {
			url = protocolScheme + url
		}
	} else {
		if !strings.HasPrefix(url, SchemeHTTP) && !strings.HasPrefix(url, SchemeHTTPS) {
			url = SchemeHTTPS + url
		}
	}

	// Parse the URL to validate it
	if _, err := netURL.Parse(url); err != nil {
		return "", err
	}

	return url, nil
}

// createPayloadReader converts various payload types to io.Reader
func createPayloadReader(payload any, opt *options.Option) (io.Reader, int64, error) {
	switch v := payload.(type) {
	case nil:
		return nil, -1, nil
	case []byte:
		opt.LogVerbose("Setting payload reader", "reader", "bytes.Reader")
		return bytes.NewReader(v), int64(len(v)), nil
	case io.Reader:
		size := int64(-1)
		if seeker, ok := v.(io.Seeker); ok {
			currentPos, _ := seeker.Seek(0, io.SeekCurrent)
			size, _ = seeker.Seek(0, io.SeekEnd)
			seeker.Seek(currentPos, io.SeekStart)
		}
		opt.LogVerbose("Setting payload reader", "reader", "io.Reader")
		return v, size, nil
	case string:
		opt.LogVerbose("Setting payload reader", "reader", "strings.Reader")
		return strings.NewReader(v), int64(len(v)), nil
	default:
		return nil, -1, fmt.Errorf("unsupported payload type: %T", payload)
	}
}
