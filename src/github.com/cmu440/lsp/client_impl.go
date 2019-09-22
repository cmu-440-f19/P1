// Contains the implementation of a LSP client.

package lsp

import "errors"

type client struct {
	// TODO: implement this!
}

// NewClient creates, initiates, and returns a new client. This function
// should return after a connection with the server has been established
// (i.e., the client has received an Ack message from the server in response
// to its connection request), and should return a non-nil error if a
// connection could not be made (i.e., if after K epochs, the client still
// hasn't received an Ack message from the server in response to its K
// connection requests).
//
// hostport is a colon-separated string identifying the server's host address
// and port number (i.e., "localhost:9999").
func NewClient(hostport string, params *Params) (Client, error) {
	return nil, errors.New("not yet implemented")
}

// ConnID returns the connection ID associated with this client.
func (c *client) ConnID() int {
	return -1
}

// Read reads a data message from the server and returns its payload.
// This method should block until data has been received from the server and
// is ready to be returned. It should return a non-nil error if either
// (1) the connection has been explicitly closed, (2) the connection has
// been lost due to an epoch timeout and no other messages are waiting to be
// returned, or (3) the server is closed. Note that in the third case, it is
// also ok for Read to never return anything.
func (c *client) Read() ([]byte, error) {
	// TODO: remove this line when you are ready to begin implementing this method.
	select {} // Blocks indefinitely.
	return nil, errors.New("not yet implemented")
}

// Write sends a data message with the specified payload to the server.
// This method should NOT block, and should return a non-nil error
// if the connection with the server has been lost. If Close has been called on
// the client, subsequent calls to Write must either return a non-nil error, or
// never return anything.
func (c *client) Write(payload []byte) error {
	return errors.New("not yet implemented")
}

// Close terminates the client's connection with the server. It should block
// until all pending messages to the server have been sent and acknowledged.
// Once it returns, all goroutines running in the background should exit.
//
// Note that after Close is called, further calls to Read, Write, and Close
// must either return a non-nil error, or never return anything.
func (c *client) Close() error {
	return errors.New("not yet implemented")
}
