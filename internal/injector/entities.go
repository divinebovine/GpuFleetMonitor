package injector

import (
	"fmt"
	"slices"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
)

// EnsureFakeEntities creates gpuCount fake GPU entities on the given
// hostengine connection if they don't already exist, and returns their
// entity IDs in a stable order. Safe to call on every process start,
// including after a crash-restart where entities from a previous
// process instance may still be present in the hostengine.
func EnsureFakeEntites(hostengineAddr string, gpuCount int) ([]uint, error) {
	cleanup, err := dcgm.Init(dcgm.Standalone, hostengineAddr, "0")
	defer cleanup()

	if err != nil {
		fmt.Printf("err initializing dcgm. err: %v\n", err)
		return []uint{}, err
	}
	fmt.Println("dcgm initialized successfully")

	fmt.Println("discovering existing fake entities...")
	discovered, err := dcgm.GetSupportedDevices()
	if err != nil {
		fmt.Printf("failed discovering existing fake entities. err: %v\n", err)
		return []uint{}, err
	}
	fmt.Printf("discovered devices: %v", discovered)

	entities := make([]dcgm.MigHierarchyInfo, 0)
	for i := range gpuCount {
		uid := uint(i)
		if !slices.Contains(discovered, uid) {
			entities = append(entities, dcgm.MigHierarchyInfo{
				Entity: dcgm.GroupEntityPair{
					EntityGroupId: dcgm.FE_GPU,
				},
			})
		}
	}

	fmt.Printf("Creating %d requested fake entities...\n", len(entities))
	created, err := dcgm.CreateFakeEntities(entities)
	if err != nil {
		return []uint{}, err
	}
	fmt.Printf("created devices: %v", created)

	out := make([]uint, 0, gpuCount)
	out = append(out, discovered...)
	out = append(out, created...)

	return out, nil
}
