package client

import (
	"fmt"
	"net/http"
	"os"

	"github.com/caelisco/http-client/v2/form"
	"github.com/caelisco/http-client/v2/options"
	"github.com/caelisco/http-client/v2/response"
)

// Client represents an HTTP client.
type Client struct {
	client    *http.Client        // HTTP client used to make requests.
	responses []response.Response // Store responses for reference.
	global    *options.Option     // Global request options applied to all requests.
}

// New returns a reusable Client.
// It is possible to include a global RequestOptions which will be used on all subsequent requests.
func New(opts ...*options.Option) *Client {
	c := &Client{
		client: &http.Client{},
	}
	// if no options are passed through, use the defaults
	c.global = options.New(opts...)
	return c
}

// NewCustom returns a reusable client with a custom defined *http.Client
// This is useful in scenarios where you want to change any configurations for the http.Client
func NewCustom(client *http.Client, opts ...*options.Option) *Client {
	c := New(opts...)
	c.client = client
	return c
}

// GetGlobalOptions returns the global RequestOptions of the client.
func (c *Client) GetGlobalOptions() *options.Option {
	return c.global
}

// AddGlobalOptions adds the provided options to the client's global options
func (c *Client) AddGlobalOptions(opts *options.Option) {
	c.global.Merge(opts)
}

// UpdateGlobalOptions updates the global RequestOptions of the client.
func (c *Client) UpdateGlobalOptions(opts *options.Option) {
	c.global = opts
}

// CloneGlobalOptions clones the global RequestOptions of the client.
func (c *Client) CloneGlobalOptions() *options.Option {
	opt := options.New()
	// Deep clone the http.Header
	opt.Header = make(http.Header)
	for key, values := range c.global.Header {
		// Make a new slice for the values
		opt.Header[key] = make([]string, len(values))
		copy(opt.Header[key], values)
	}

	// Deep clone http.Cookies
	opt.Cookies = make([]*http.Cookie, len(c.global.Cookies))
	for i, cookie := range c.global.Cookies {
		opt.Cookies[i] = &http.Cookie{
			Name:       cookie.Name,
			Value:      cookie.Value,
			Path:       cookie.Path,
			Domain:     cookie.Domain,
			Expires:    cookie.Expires,
			RawExpires: cookie.RawExpires,
			MaxAge:     cookie.MaxAge,
			Secure:     cookie.Secure,
			HttpOnly:   cookie.HttpOnly,
			SameSite:   cookie.SameSite,
			Raw:        cookie.Raw,
			Unparsed:   append([]string{}, cookie.Unparsed...),
		}
	}

	return opt
}

// Clear clears any Responses that have already been made and kept.
func (c *Client) Clear() {
	c.responses = nil
}

// Responses returns a slice of responses made by this Client
func (c *Client) Responses() []response.Response {
	return c.responses
}

func (c *Client) doRequest(method string, url string, payload any, opts ...*options.Option) (response.Response, error) {
	// Clone global options so that we do not overwrite them with each subsequent request
	opt := options.New(opts...)
	opt.Merge(c.CloneGlobalOptions())
	opt.SetClient(c.client)
	// Perform the request with the merged options
	response, err := doRequest(method, url, payload, opt)

	// Keep the response
	c.responses = append(c.responses, response)
	return response, err
}

// Get performs an HTTP GET to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Get(url string, opts ...*options.Option) (response.Response, error) {
	return c.doRequest(http.MethodGet, url, nil, opts...)
}

// Post performs an HTTP POST to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Post(url string, payload any, opts ...*options.Option) (response.Response, error) {
	return c.doRequest(http.MethodPost, url, payload, opts...)
}

// PostFormData performs an HTTP POST as an x-www-form-urlencoded payload to the specified URL.
// It accepts the URL string as its first argument and a map[string]string the payload.
// The map is converted to a url.QueryEscaped k/v pair that is sent to the server.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) PostFormData(url string, payload map[string]string, opts ...*options.Option) (response.Response, error) {
	opt := &options.Option{}
	if len(opts) > 0 {
		opt.Merge(opts[0])
	}
	opt.AddHeader(ContentType, "application/x-www-form-urlencoded")

	return c.Post(url, form.Encode(payload), opt)
}

// PostFile uploads a file to the specified URL using an HTTP POST request.
// It accepts the URL string as its first argument and the filename as the second argument.
// The file is read from the specified filename and uploaded as the request payload.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) PostFile(url string, filename string, opts ...*options.Option) (response.Response, error) {
	_, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return response.Response{}, fmt.Errorf("file does not exist: %s", filename)
		}
		return response.Response{}, fmt.Errorf("failed to access file: %v", err)
	}

	file, err := os.Open(filename)
	if err != nil {
		return response.Response{}, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Use the Post method to send the file
	return c.Post(url, file, opts...)
}

// Put performs an HTTP PUT to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Put(url string, payload any, opts ...*options.Option) (response.Response, error) {
	return c.doRequest(http.MethodPut, url, payload, opts...)
}

// PutFormData performs an HTTP PUT as an x-www-form-urlencoded payload to the specified URL.
// It accepts the URL string as its first argument and a map[string]string the payload.
// The map is converted to a url.QueryEscaped k/v pair that is sent to the server.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) PutFormData(url string, payload map[string]string, opts ...*options.Option) (response.Response, error) {
	opt := &options.Option{}
	if len(opts) > 0 {
		opt.Merge(opts[0])
	}
	opt.AddHeader(ContentType, "application/x-www-form-urlencoded")

	return c.Put(url, form.Encode(payload), opt)
}

// PutFile uploads a file to the specified URL using an HTTP PUT request.
// It accepts the URL string as its first argument and the filename as the second argument.
// The file is read from the specified filename and uploaded as the request payload.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) PutFile(url string, filename string, opts ...*options.Option) (response.Response, error) {
	_, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return response.Response{}, fmt.Errorf("file does not exist: %s", filename)
		}
		return response.Response{}, fmt.Errorf("failed to access file: %v", err)
	}

	file, err := os.Open(filename)
	if err != nil {
		return response.Response{}, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Use the Post method to send the file
	return c.Put(url, file, opts...)
}

// Patch performs an HTTP PATCH to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Patch(url string, payload any, opts ...*options.Option) (response.Response, error) {
	return c.doRequest(http.MethodPatch, url, payload, opts...)
}

// PatchFormData performs an HTTP PATCH as an x-www-form-urlencoded payload to the specified URL.
// It accepts the URL string as its first argument and a map[string]string the payload.
// The map is converted to a url.QueryEscaped k/v pair that is sent to the server.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) PatchFormData(url string, payload map[string]string, opts ...*options.Option) (response.Response, error) {
	opt := &options.Option{}
	if len(opts) > 0 {
		opt.Merge(opts[0])
	}
	opt.AddHeader(ContentType, "application/x-www-form-urlencoded")

	return c.Patch(url, form.Encode(payload), opt)
}

// PatchFile uploads a file to the specified URL using an HTTP PATCH request.
// It accepts the URL string as its first argument and the filename as the second argument.
// The file is read from the specified filename and uploaded as the request payload.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) PatchFile(url string, filename string, opts ...*options.Option) (response.Response, error) {
	_, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return response.Response{}, fmt.Errorf("file does not exist: %s", filename)
		}
		return response.Response{}, fmt.Errorf("failed to access file: %v", err)
	}

	file, err := os.Open(filename)
	if err != nil {
		return response.Response{}, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Use the Post method to send the file
	return c.Patch(url, file, opts...)
}

// Delete performs an HTTP DELETE to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Delete(url string, opts ...*options.Option) (response.Response, error) {
	return c.doRequest(http.MethodDelete, url, nil, opts...)
}

// Connect performs an HTTP CONNECT to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Connect(url string, opts ...*options.Option) (response.Response, error) {
	return c.doRequest(http.MethodConnect, url, nil, opts...)
}

// Head performs an HTTP HEAD to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Head(url string, opts ...*options.Option) (response.Response, error) {
	return c.doRequest(http.MethodHead, url, nil, opts...)
}

// Options performs an HTTP OPTIONS to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Options(url string, opts ...*options.Option) (response.Response, error) {
	return c.doRequest(http.MethodHead, url, nil, opts...)
}

// Trace performs an HTTP TRACE to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Trace(url string, opts ...*options.Option) (response.Response, error) {
	return c.doRequest(http.MethodTrace, url, nil, opts...)
}

// Custom performs a custom HTTP method to the specified URL with the given payload.
// It accepts the HTTP method as its first argument, the URL string as the second argument,
// the payload as the third argument, and optionally additional Options to customize the request.
// Returns the HTTP response and an error if any.
func (c *Client) Custom(method string, url string, payload any, opts ...*options.Option) (response.Response, error) {
	return c.doRequest(method, url, payload, opts...)
}
