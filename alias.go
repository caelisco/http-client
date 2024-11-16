package client

import (
	"github.com/caelisco/http-client/request"
	"github.com/caelisco/http-client/response"
)

// In changing between v0.1.0 and v0.2.0 there was a lot of code reorganisation.
// For backward compatablility, alias are introduced.

// Alias to request.Options
type RequestOptions = request.Options

// Alias to response.Response
type Response = response.Response
