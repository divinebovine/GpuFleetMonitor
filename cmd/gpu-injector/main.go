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

/*
  Step 5 — build and run against a fresh spike hostengine to confirm it actually
  links/runs in-container, not just on your host:
  docker run -d --name dcgm-spike nvidia/dcgm:4.5.2-1-ubuntu22.04 -n -b 0.0.0.0
  --log-level INFO -f -
  docker build -f hack/demo/Dockerfile.gpu-injector -t gpu-injector:spike .
  docker run --rm --network container:dcgm-spike gpu-injector:spike
  docker rm -f dcgm-spike
*/
