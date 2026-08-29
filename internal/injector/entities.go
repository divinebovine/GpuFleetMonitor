package injector

import (
	"fmt"
	"log/slog"
	"slices"

	"github.com/NVIDIA/go-dcgm/pkg/dcgm"
)

// EnsureFakeEntities creates gpuCount fake GPU entities on the given
// hostengine connection if they don't already exist, and returns their
// entity IDs in a stable order. Safe to call on every process start,
// including after a crash-restart where entities from a previous
// process instance may still be present in the hostengine.
func EnsureFakeEntites(gpuCount int) ([]uint, error) {
	slog.Info("discovering existing fake entities...")
	discovered, err := dcgm.GetSupportedDevices()
	if err != nil {
		slog.Error("failed discovering existing fake entities", "error", err)
		return []uint{}, err
	}
	slog.Info("discovered devices", "devices", discovered)

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

	slog.Info(fmt.Sprintf("Creating %d requested fake entities...", len(entities)))
	created, err := dcgm.CreateFakeEntities(entities)
	if err != nil {
		return []uint{}, err
	}
	slog.Info("created devices", "created", created)

	out := make([]uint, 0, gpuCount)
	out = append(out, discovered...)
	out = append(out, created...)

	return out, nil
}
