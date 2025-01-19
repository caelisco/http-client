package client

import (
	"fmt"
	netURL "net/url"
	"strings"
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
