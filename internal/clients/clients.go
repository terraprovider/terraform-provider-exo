// Package clients holds the configured Admin API client shared by all resources.
package clients

import (
	"github.com/terraprovider/go-exoscc/adminapi"
	"github.com/terraprovider/go-exoscc/exo"
)

// Client is passed to every resource/data source via Configure.
type Client struct {
	Admin    *adminapi.Client
	EXO      *exo.Service
	TenantID string
}
