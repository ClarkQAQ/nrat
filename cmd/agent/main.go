package main

import (
	"uw/uboot"
	"uw/ulog"

	"nrat/cmd/agent/internal/agent"
	"nrat/cmd/agent/internal/storage"
	"nrat/pkg/unostr"

	"github.com/abiosoft/readline"
)

func main() {
	ulog.GlobalFormat().SetWriter(func(s string) {
		readline.Stdout.Write([]byte(s))
	})

	uboot.NewBoot().Register(
		uboot.Uint("storage", uboot.UintNormal, storage.StorageUint),
		uboot.Uint("unostr", uboot.UintNormal, unostr.UnostrUint),
		uboot.Uint("agent", uboot.UintDaemon, agent.AgentUint),
	).BootTimeout(0).Start()
}
