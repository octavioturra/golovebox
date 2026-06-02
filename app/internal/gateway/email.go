package gateway

import "errors"

// EmailHandler is a placeholder for the future email gateway.
type EmailHandler struct{}

// NewEmailHandler always returns an error — email gateway is not implemented in V0.
func NewEmailHandler(_ *Gateway) (*EmailHandler, error) {
	return nil, errors.New("email gateway: not implemented in V0")
}
