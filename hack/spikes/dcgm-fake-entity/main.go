package main

import (
	"fmt"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
)

func main() {
	cleanup, err := dcgm.Init(dcgm.Standalone, "127.0.0.1:5555", "0")
	defer cleanup()

	if err != nil {
		fmt.Printf("err initializing dcgm. err: %v", err)
	}
	fmt.Println("dcgm initialized successfully")

	ids, err := dcgm.CreateFakeEntities([]dcgm.MigHierarchyInfo{
		{Entity: dcgm.GroupEntityPair{EntityGroupId: dcgm.FE_GPU}},
	})

	if err != nil {
		fmt.Printf("failed creating fake entity. err: %v", err)
	}
	fmt.Printf("created fake enttities: %v\n", ids)

	err = dcgm.InjectFieldValue(ids[0], dcgm.DCGM_FI_DEV_GPU_TEMP_CELSIUS, dcgm.DCGM_FT_INT64, 0, 0, int64(65))

	if err != nil {
		fmt.Printf("failed injecting field value. err: %v", err)
	}

	fmt.Printf("injected temp=65 on entity %d\n", ids[0])
}
