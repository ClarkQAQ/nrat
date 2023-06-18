package model

import (
	"time"

	"nrat/pkg/nostr"
)

type Unostr interface {
	Connect() error
	SetConnectEvent(f func())
	Relay() *nostr.Relay
	Close() error
	ConnectTimeout() time.Duration
}
