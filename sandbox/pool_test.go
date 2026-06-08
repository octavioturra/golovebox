package sandbox

import (
	"sync"
	"testing"
)

// fakeSSHClient is a minimal stand-in for *ssh.Client in pool tests.
// We test pool structure directly to avoid the real SSH dependency.

func TestPool_storesAndReturnsConfig(t *testing.T) {
	cfg := PoolConfig{Host: "127.0.0.1", Port: 2222, User: "root", KeyPath: "/tmp/id_rsa"}
	p := NewPool(cfg)
	if p.cfg != cfg {
		t.Fatalf("NewPool stored wrong config: %+v", p.cfg)
	}
}

func TestPool_closeEmptyPoolIsNoOp(t *testing.T) {
	p := NewPool(PoolConfig{Host: "127.0.0.1", Port: 2222, User: "root"})
	if err := p.Close(); err != nil {
		t.Fatalf("Close on empty pool: %v", err)
	}
}

func TestPool_concurrentCloseIsSafe(t *testing.T) {
	p := NewPool(PoolConfig{})
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.Close()
		}()
	}
	wg.Wait()
}
