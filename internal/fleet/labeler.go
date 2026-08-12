package fleet

import "errors"

const (
	LabelModel = "gpu.fleet/model"
	LabelCount = "gpu.fleet/count"
)

var (
	ErrTooFewNodes  = errors.New("Too few nodes")
	ErrTooManyNodes = errors.New("Too many nodes")
)

type Assignment struct {
	NodeName string
	Model    string
	GPUCount int
}

func AssignGroups(nodeNames []string, groups []NodeGroup) ([]Assignment, error) {
	var nodeCount int
	for _, g := range groups {
		nodeCount += g.NodeCount
	}

	if len(nodeNames) > nodeCount {
		return nil, ErrTooFewNodes
	}
	if len(nodeNames) < nodeCount {
		return nil, ErrTooManyNodes
	}

	out := make([]Assignment, 0)
	gIdx := 0
	assigned := 0
	for i := range nodeCount {
		out = append(out, Assignment{
			NodeName: nodeNames[i],
			Model:    groups[gIdx].Model,
			GPUCount: groups[gIdx].GPUCount,
		})
		assigned++

		if groups[gIdx].NodeCount == assigned {
			assigned = 0
			gIdx++
		}
	}

	return out, nil
}
