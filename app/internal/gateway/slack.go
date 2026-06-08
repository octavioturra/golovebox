package gateway

import "errors"

// SlackHandler is a placeholder for the future Slack gateway.
type SlackHandler struct{}

// NewSlackHandler always returns an error — Slack gateway is not implemented in V0.
func NewSlackHandler(_ string, _ *Gateway) (*SlackHandler, error) {
	return nil, errors.New("slack gateway: not implemented in V0")
}
