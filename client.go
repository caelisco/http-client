package client

import (
	"net/http"

	"github.com/caelisco/http-client/form"
	"github.com/caelisco/http-client/kv"
	"github.com/caelisco/http-client/request"
)

// Client represents an HTTP client.
type Client struct {
	client    *http.Client   // HTTP client used to make requests.
	responses []Response     // Store responses for reference.
	global    RequestOptions // Global request options applied to all requests.
}

// New returns a reusable Client.
// It is possible to include a global RequestOptions which will be used on all subsequent requests.
func New(options ...RequestOptions) *Client {
	c := &Client{
		client: &http.Client{},
	}
	// if no options are passed through, use the defaults
	if len(options) == 0 {
		c.global = request.NewOptions()
	} else {
		c.global = options[0]
	}
	return c
}

// NewCustom returns a reusable client with a custom defined *http.Client
// This is useful in scenarios where you want to change any configurations for the http.Client
func NewCustom(client *http.Client, options ...RequestOptions) *Client {
	c := New(options...)
	c.client = client
	return c
}

// GetGlobalOptions returns the global RequestOptions of the client.
func (c *Client) GetGlobalOptions() RequestOptions {
	return c.global
}

// AddGlobalOptions adds the provided options to the client's global options
func (c *Client) AddGlobalOptions(options RequestOptions) {
	c.global.Merge(options)
}

// UpdateGlobalOptions updates the global RequestOptions of the client.
func (c *Client) UpdateGlobalOptions(options RequestOptions) {
	c.global = options
}

// CloneGlobalOptions clones the global RequestOptions of the client.
func (c *Client) CloneGlobalOptions() RequestOptions {
	opt := RequestOptions{}
	// Create a new slice and copy the elements to the new slice
	opt.Headers = make([]kv.Header, len(c.global.Headers))
	copy(opt.Headers, c.global.Headers)
	opt.Cookies = make([]*http.Cookie, len(c.global.Cookies))
	copy(opt.Cookies, c.global.Cookies)

	return opt
}

// Clear clears any Responses that have already been made and kept.
func (c *Client) Clear() {
	c.responses = nil
}

// Responses returns a slice of responses made by this Client
func (c *Client) Responses() []Response {
	return c.responses
}

func (c *Client) doRequest(method string, url string, payload []byte, options ...RequestOptions) (Response, error) {
	// Clone global options so that we do not overwrite them with each subsequent request
	opt := c.CloneGlobalOptions()

	// Merge the local RequestOptions with the global RequestOptions
	if len(options) == 0 {
		opt = request.NewOptions()
	} else {
		opt.Merge(options[0])
	}

	// Perform the request with the merged options
	response, err := doRequest(c.client, method, url, payload, opt)

	// Keep the response
	c.responses = append(c.responses, response)
	return response, err
}

// Get performs an HTTP GET to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Get(url string, opt ...RequestOptions) (Response, error) {
	return c.doRequest(http.MethodGet, url, nil, opt...)
}

// Post performs an HTTP POST to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Post(url string, payload []byte, opt ...RequestOptions) (Response, error) {
	return c.doRequest(http.MethodPost, url, payload, opt...)
}

// FormPost performs an HTTP POST as an x-www-form-urlencoded payload to the specified URL.
// It accepts the URL string as its first argument and a map[string]string the payload.
// The map is converted to a url.QueryEscaped k/v pair that is sent to the server.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) FormData(url string, payload map[string]string, opt ...RequestOptions) (Response, error) {
	switch len(opt) {
	case 0:
		option := RequestOptions{}
		option.AddHeader("Content-Type", "application/x-www-form-urlencoded")
		opt = append(opt, option)
	case 1:
		opt[0].AddHeader("Content-Type", "application/x-www-form-urlencoded")
	}
	return c.doRequest(http.MethodPost, url, form.Encode(payload), opt...)
}

// Put performs an HTTP PUT to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Put(url string, payload []byte, opt ...RequestOptions) (Response, error) {
	return c.doRequest(http.MethodPut, url, payload, opt...)
}

// Patch performs an HTTP PATCH to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Patch(url string, payload []byte, opt ...RequestOptions) (Response, error) {
	return c.doRequest(http.MethodPatch, url, payload, opt...)
}

// Delete performs an HTTP DELETE to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Delete(url string, opt ...RequestOptions) (Response, error) {
	return c.doRequest(http.MethodDelete, url, nil, opt...)
}

// Connect performs an HTTP CONNECT to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Connect(url string, opt ...RequestOptions) (Response, error) {
	return c.doRequest(http.MethodConnect, url, nil, opt...)
}

// Head performs an HTTP HEAD to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Head(url string, opt ...RequestOptions) (Response, error) {
	return c.doRequest(http.MethodHead, url, nil, opt...)
}

// Options performs an HTTP OPTIONS to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Options(url string, opt ...RequestOptions) (Response, error) {
	return c.doRequest(http.MethodHead, url, nil, opt...)
}

// Trace performs an HTTP TRACE to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Trace(url string, opt ...RequestOptions) (Response, error) {
	return c.doRequest(http.MethodTrace, url, nil, opt...)
}

// Custom performs a custom HTTP method to the specified URL with the given payload.
// It accepts the HTTP method as its first argument, the URL string as the second argument,
// the payload as the third argument, and optionally additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Custom(method string, url string, payload []byte, opt ...RequestOptions) (Response, error) {
	return c.doRequest(method, url, payload, opt...)
}
