package form

import (
	"net/url"
	"strings"
)

func Encode(m map[string]string) []byte {
	// Build the form data query string
	var encoded []string
	for k, v := range m {
		key := url.QueryEscape(k)
		value := url.QueryEscape(v)
		encoded = append(encoded, key+"="+value)
	}
	return []byte(strings.Join(encoded, "&"))
}
