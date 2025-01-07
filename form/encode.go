package form

import (
	"net/url"
	"strings"
)

// Encode encodes the provided map[string]string as application/x-www-form-urlencoded form data.
// Each key and value in the map is URL-encoded and concatenated into a query string format.
// The resulting query string is returned as a byte slice, suitable for use as the body of a POST request.
func Encode(m map[string]string) []byte {
	var encoded []string
	for k, v := range m {
		key := url.QueryEscape(k)
		value := url.QueryEscape(v)
		encoded = append(encoded, key+"="+value)
	}
	return []byte(strings.Join(encoded, "&"))
}
