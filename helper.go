package client

import (
	netURL "net/url"
	"strings"
)

func normaliseURL(url string, protocolScheme string) (string, error) {
	url = strings.TrimSpace(url)

	if protocolScheme != "" {
		// Clean the protocol scheme prior to adding the new one
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
