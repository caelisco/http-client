# http-client
A small wrapper around the HTTP Client in Go.

The client is not necessarily feature complete and is designed to work with simple requests to an HTTP server.

It has two modes of operation: 
1. Normal HTTP method function calls
2. A reusable client

In most cases, the HTTP method function calls are probably what you are looking for.

The reusable client is typically more useful in scenarios where you need to work with cookies, or in scenarios where it will be useful to keep track of past requests. An example could be for writing an API test that might require different headers/cookies to be set and used as part of the testing of each end point.

HTTP method calls use the http.DefaultClient, while the reusable client establishes its own internal *http.Client which provides additional 