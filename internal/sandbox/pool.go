package sandbox

import (
	"context"
	"strconv"
	"sync"

	"golang.org/x/crypto/ssh"
)

// PoolConfig holds the parameters needed to create new SSH connections.
type PoolConfig struct {
	Host    string
	Port    int
	User    string
	KeyPath string
}

// Pool maintains a set of idle SSH connections and creates new ones on demand.
// Connections are reused across tool calls to avoid per-call handshake overhead.
type Pool struct {
	mu   sync.Mutex
	idle []*ssh.Client
	cfg  PoolConfig
}

// NewPool creates a pool with the given connection parameters.
func NewPool(cfg PoolConfig) *Pool {
	return &Pool{cfg: cfg}
}

// Acquire returns an idle connection from the pool, or dials a new one.
// Idle connections are validated with a keepalive before being returned;
// stale connections are discarded and the next one is tried.
// Respects ctx cancellation — returns ctx.Err() if the context is done before a
// new connection is established.
func (p *Pool) Acquire(ctx context.Context) (*ssh.Client, error) {
	for {
		p.mu.Lock()
		if len(p.idle) == 0 {
			p.mu.Unlock()
			break
		}
		c := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]
		p.mu.Unlock()

		_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
		if err == nil {
			return c, nil
		}
		_ = c.Close()
		// stale connection discarded — try next idle or dial fresh
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return Dial(p.cfg.Host, strconv.Itoa(p.cfg.Port), p.cfg.User, p.cfg.KeyPath)
}

// Release returns a connection to the pool for reuse.
func (p *Pool) Release(c *ssh.Client) {
	p.mu.Lock()
	p.idle = append(p.idle, c)
	p.mu.Unlock()
}

// Close closes all idle connections in the pool.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.idle {
		_ = c.Close()
	}
	p.idle = nil
	return nil
}
