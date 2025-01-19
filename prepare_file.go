package client

import (
	"fmt"
	"os"

	"github.com/caelisco/http-client/options"
)

func prepareFile(filename string, opts ...*options.Option) (*os.File, *options.Option, error) {
	fileinfo, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("file does not exist: %s", filename)
		}
		return nil, nil, fmt.Errorf("failed to access file: %v", err)
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open file: %v", err)
	}

	opt := options.New(opts...)
	opt.InferContentType(file, fileinfo)

	return file, opt, nil
}
