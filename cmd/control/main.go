package main

import (
	"uw/uboot"

	"nrat/cmd/control/internal/control"
	"nrat/cmd/control/internal/storage"
	"nrat/pkg/unostr"
)

func main() {
	uboot.NewBoot().Register(
		uboot.Uint("storage", uboot.UintNormal, storage.StorageUint),
		uboot.Uint("unostr", uboot.UintNormal, unostr.UnostrUint),
		uboot.Uint("control", uboot.UintAfter, control.ControlUint),
	).BootTimeout(0).Start()
}
