package main

import (
	"fmt"
	"log"

	client "github.com/caelisco/http-client"
	"github.com/caelisco/http-client/request"
)

func main() {
	c := client.New()
	opt := request.NewOptions()
	// set the identifier to ULID
	opt.UniqueIdentifier = request.IdentifierUUID

	// The options added to the request are only for this request.
	// The resp.UniqueIdentifier will be a UUID
	resp, err := c.Get("https://www.caelisco.net/", opt)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Received information from", resp.URL, "with internal ID:", resp.UniqueIdentifier, "body length:", resp.Length(), "content-length:", resp.ContentLength)

	// perform a second request
	// With no options provided, it uses the default options. The resp.UniqueIdentifier will be a ULID
	resp, err = c.Get("https://www.caelisco.net/")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Received information from", resp.URL, "with internal ID:", resp.UniqueIdentifier, "body length:", resp.Length(), "content-length:", resp.ContentLength)

}
