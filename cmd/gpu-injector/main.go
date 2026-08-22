package main

import (
	"fmt"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
)

func main() {
	cleanup, err := dcgm.Init(dcgm.Standalone, "127.0.0.1:5555", "0")
	defer cleanup()

	if err != nil {
		fmt.Printf("err initializing dcgm. err: %v\n", err)
		return
	}
	fmt.Println("dcgm initialized successfully")

	devices, err := dcgm.GetSupportedDevices()
	if err != nil {
		fmt.Printf("err getting supported devices. err: %v\n", err)
		return
	}
	fmt.Printf("Discovered %d supported devices.\n", len(devices))
}
