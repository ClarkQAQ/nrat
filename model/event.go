package model

import (
	"fmt"
	"strings"
)

const (
	EventSeparator = "\x1e"
	DataSeparator  = "\x1f"
)

type Event struct {
	Id      string // 编号
	Type    string // 事件类型
	Error   string // 错误消息
	Content string // 事件内容
}

func (evt *Event) Encode() string {
	return evt.Type + EventSeparator + evt.Error + EventSeparator + evt.Content
}

func (evt *Event) Decode(t string) error {
	n := strings.SplitN(t, EventSeparator, 3)
	if len(n) < 3 {
		return fmt.Errorf("invalid event: %s", t)
	}

	evt.Type, evt.Error, evt.Content = n[0], n[1], n[2]
	return nil
}
