package sandbox

import (
	"encoding/json"
	"fmt"
	"time"

	goqump "github.com/digitalocean/go-qemu/qmp"
)

type QMPClient struct {
	mon *goqump.SocketMonitor
}

func Connect(addr string) (*QMPClient, error) {
	mon, err := goqump.NewSocketMonitor("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("qmp new monitor %s: %w", addr, err)
	}
	if err := mon.Connect(); err != nil {
		return nil, fmt.Errorf("qmp connect %s: %w", addr, err)
	}
	return &QMPClient{mon: mon}, nil
}

func (c *QMPClient) Quit() error {
	_, err := c.mon.Run([]byte(`{"execute":"quit"}`))
	return err
}

func (c *QMPClient) Status() (string, error) {
	resp, err := c.mon.Run([]byte(`{"execute":"query-status"}`))
	if err != nil {
		return "", err
	}
	var result struct {
		Return struct {
			Status string `json:"status"`
		} `json:"return"`
	}
	if jsonErr := json.Unmarshal(resp, &result); jsonErr != nil {
		return string(resp), nil
	}
	return result.Return.Status, nil
}

func (c *QMPClient) Disconnect() error {
	return c.mon.Disconnect()
}
