package rpc

import (
	"encoding/json"
	"errors"
	"io"
	"sync"
)

// Client is a small Go client for a gator rpc child process. It is safe for
// one goroutine to receive messages while other goroutines submit commands.
type Client struct {
	encoder *json.Encoder
	decoder *json.Decoder
	write   sync.Mutex
}

// NewClient connects protocol input and output. Output is written to Gator's
// stdin; input is read from Gator's stdout.
func NewClient(input io.Reader, output io.Writer) (*Client, error) {
	if input == nil || output == nil {
		return nil, errors.New("RPC client input and output are required")
	}
	return &Client{encoder: json.NewEncoder(output), decoder: json.NewDecoder(input)}, nil
}

// Send writes one command. The current protocol version is inserted when a
// caller leaves Version at zero; explicit incompatible versions are sent as
// supplied so a server can give the caller a useful compatibility error.
func (c *Client) Send(request Request) error {
	if c == nil || c.encoder == nil {
		return errors.New("RPC client is not initialized")
	}
	if request.Version == 0 {
		request.Version = Version
	}
	c.write.Lock()
	defer c.write.Unlock()
	return c.encoder.Encode(request)
}

// Receive returns the next response, event, or error. io.EOF means the Gator
// child closed its output and is not converted into a protocol error.
func (c *Client) Receive() (Message, error) {
	if c == nil || c.decoder == nil {
		return Message{}, errors.New("RPC client is not initialized")
	}
	var message Message
	if err := c.decoder.Decode(&message); err != nil {
		return Message{}, err
	}
	return message, nil
}
