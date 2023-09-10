package unostr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"strings"
	"time"
	"uw/uboot"
	"uw/ulog"

	"nrat/pkg/nostr"

	"nrat/model"
	"nrat/utils"

	"golang.org/x/net/proxy"
)

func UnostrUint(c *uboot.Context) error {
	if e := c.Require(c.Context(), "storage"); e != nil {
		return e
	}

	storage, ok := utils.UbootGetAssert[model.UnostrStorage](c, "storage")
	if !ok {
		return errors.New("get storage failed")
	}

	connectTimeout, e := time.ParseDuration(storage.Unostr().ConnectTimeout)
	if e != nil || connectTimeout < 1 {
		ulog.Warn("parse connect timeout failed or connect timeout < 1, use default 5s")
		connectTimeout = 5 * time.Second
	}
	storage.Unostr().ConnectTimeout = connectTimeout.String()

	pingInterval, e := time.ParseDuration(storage.Unostr().PingInterval)
	if e != nil || pingInterval < 1 {
		ulog.Warn("parse ping interval failed or ping interval < 1, use default 10s")
		pingInterval = 10 * time.Second
	}
	storage.Unostr().PingInterval = pingInterval.String()

	if e := storage.Write(); e != nil {
		return fmt.Errorf("write storage failed: %w", e)
	}

	u := &Unostr{
		relayURL:       strings.TrimSpace(storage.Unostr().Relay),
		proxyURL:       strings.TrimSpace(storage.Unostr().Proxy),
		connectTimeout: connectTimeout,
	}

	if u.relayURL == "" {
		return errors.New("relay is empty")
	}

	c.Printf("relay: %s", u.relayURL)

	u.relay = nostr.NewRelay(context.Background(), u.relayURL)

	nostr.InfoLogger = log.New(io.Discard, "", log.LstdFlags)
	nostr.DebugLogger = log.New(io.Discard, "", log.LstdFlags)

	if u.proxyURL != "" {
		c.Printf("use proxy: %s", u.proxyURL)

		proxyURL, e := url.Parse(u.proxyURL)
		if e != nil {
			return fmt.Errorf("failed to parse proxy url: %w", e)
		}

		dialer, e := proxy.FromURL(proxyURL, proxy.Direct)
		if e != nil {
			return fmt.Errorf("failed to create dialer: %w", e)
		}

		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return fmt.Errorf("failed to convert dialer to context dialer")
		}

		u.relay.Dial = contextDialer.DialContext
	}

	c.Printf("connect timeout: %s", u.connectTimeout)

	c.Printf("first connecting to %s...", u.relayURL)

	for {
		if e := u.Connect(); e != nil {
			c.Printf("first connect to %s failed: %s", u.relayURL, e)

			c.Printf("retrying after %s", u.connectTimeout)
			<-time.After(u.connectTimeout)
			c.Printf("ready retrying connect to %s", u.relayURL)
			continue
		}

		break
	}

	c.Printf("first connected to %s", u.relayURL)

	go u.pingLoop(pingInterval)

	c.Set("unostr", u)
	return nil
}

type Unostr struct {
	relayURL string // 中继器
	proxyURL string // 代理

	relay *nostr.Relay

	connectTimeout time.Duration // 连接超时
	pingTicker     *time.Ticker
	connectEvent   func()
}

func (u *Unostr) ConnectTimeout() time.Duration {
	return u.connectTimeout
}

func (u *Unostr) Connect() error {
	if u.relay.Connection != nil {
		u.relay.Connection.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), u.connectTimeout)
	defer cancel()

	if e := u.relay.Connect(ctx); e != nil {
		return e
	}

	u.relay.Connection.SetRawLog(ulog.Debug)

	if u.connectEvent != nil {
		u.connectEvent()
	}

	return nil
}

func (u *Unostr) SetConnectEvent(f func()) {
	u.connectEvent = f
}

func (u *Unostr) Relay() *nostr.Relay {
	return u.relay
}

func (u *Unostr) Close() error {
	if u.pingTicker != nil {
		u.pingTicker.Stop()
	}

	if u.relay.Connection != nil {
		if e := u.relay.Connection.Close(); e != nil {
			return e
		}
	}

	if e := u.relay.Close(); e != nil {
		return e
	}

	return nil
}

func (u *Unostr) pingLoop(pingInterval time.Duration) {
	if u.pingTicker != nil {
		u.pingTicker.Stop()
	}

	u.pingTicker = time.NewTicker(pingInterval)
	for range u.pingTicker.C {
		if u.relay.Connection != nil {
			if u.relay.Connection.Ping() == nil {
				// ulog.Debug("ping %s success", u.relayURL)
				continue
			}
		}

		ulog.Warn("reconnect to %s", u.relayURL)

		if e := u.Connect(); e != nil {
			ulog.Error("failed to reconnect to %s: %s", u.relayURL, e.Error())
		}
	}
}
