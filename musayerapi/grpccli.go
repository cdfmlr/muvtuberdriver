// ## gRPC Client: musayerapi/grpccli.go
//
// There are two kinds of SayerService clients:
//
//  1. SayerClient: a gRPC client for the SayerService
//  2. SayerClientPool: a pool of SayerClients
//
// Both SayerClient and SayerClientPool implement the Sayer interface.
// So they can simply be used as Sayers. (bravo decorator & composite pattern!)
package musayerapi

import (
	"context"

	"google.golang.org/grpc"

	"muvtuberdriver/pkg/pool"
	sayerv1 "muvtuberdriver/musayerapi/proto"
)

var MaxConsecutiveFailures = 3

// SayerClient is a gRPC client for the SayerService
type SayerClient struct {
	client sayerv1.SayerServiceClient

	pool.Poolable
	failed int // successive failures of Say. SayerClient.Say mantains this value, and should not be modified by other code.
}

// NewSayerClient creates a new SayerClient
func NewSayerClient(addr string) (*SayerClient, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return &SayerClient{
		client: sayerv1.NewSayerServiceClient(conn),
	}, nil
}

// Say calls the Say method of the SayerService
//
// Say implements the Sayer interface. So SayerClient can be used
// as a Sayer.
func (c *SayerClient) Say(role string, text string) (format string, audio []byte, err error) {
	resp, err := c.client.Say(context.Background(), &sayerv1.SayRequest{
		Role: role,
		Text: text,
	})
	if err != nil {
		c.failed++
		return "", nil, err
	}
	c.failed = 0
	return resp.Format, resp.Audio, nil
}

// Close closes the SayerClient
func (c *SayerClient) Close() error {
	return nil
}

// SayerClientPool is a pool of SayerClients.
//
// SayerClientPool implements the Sayer interface.
// So SayerClientPool can be used as a Sayer.
type SayerClientPool struct {
	pool pool.Pool[*SayerClient]
}

func NewSayerClientPool(addr string, size int64) (*SayerClientPool, error) {
	p := pool.NewPool(size, func() (*SayerClient, error) {
		return NewSayerClient(addr)
	})

	return &SayerClientPool{
		pool: p,
	}, nil
}

func (p *SayerClientPool) Say(role string, text string) (format string, audio []byte, err error) {
	// get a client from the pool
	client, err := p.pool.Get()
	if err != nil {
		return "", nil, err
	}

	// call Say on the client
	format, audio, err = client.Say(role, text)
	if err != nil && client.failed > MaxConsecutiveFailures {
		// if the client has failed MaxConsecutiveFailures times in a row, remove it from the pool
		p.pool.Release(client)
	} else {
		// otherwise, put it back into the pool
		p.pool.Put(client)
	}

	return format, audio, nil
}
