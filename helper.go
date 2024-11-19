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

// mergeOptions merges source options into target options, with source taking priority
func mergeOptions(dest *RequestOptions, src RequestOptions) {

	// Merge headers
	for _, sh := range src.Headers {
		found := false
		for i, th := range dest.Headers {
			if th.Key == sh.Key {
				dest.Headers[i] = sh
				found = true
				break
			}
		}
		if !found {
			dest.Headers = append(dest.Headers, sh)
		}
	}

	// Merge cookies
	for _, sc := range src.Cookies {
		found := false
		for i, tc := range dest.Cookies {
			if tc.Name == sc.Name {
				dest.Cookies[i] = sc
				found = true
				break
			}
		}
		if !found {
			dest.Cookies = append(dest.Cookies, sc)
		}
	}

	// Merge other fields, source takes priority if not empty
	if src.UniqueIdentifier != "" {
		dest.UniqueIdentifier = src.UniqueIdentifier
	}
	if src.Compression != "" {
		dest.Compression = src.Compression
	}
	if src.UserAgent != "" {
		dest.UserAgent = src.UserAgent
	}
	if src.ProtocolScheme != "" {
		dest.ProtocolScheme = src.ProtocolScheme
	}
	// DisableRedirect is a boolean, so we always take the source value
	dest.DisableRedirect = src.DisableRedirect
	if src.Writer != nil {
		dest.Writer = src.Writer
	}
}
