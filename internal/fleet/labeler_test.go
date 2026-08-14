package fleet

import (
	"errors"
	"slices"
	"testing"
)

func assignmentsAreEqual(a, b []Assignment) bool {
	return slices.EqualFunc(a, b, func(a1, a2 Assignment) bool {
		return a1.GPUCount == a2.GPUCount && a1.Model == a2.Model && a1.NodeName == a2.NodeName
	})
}

const (
	h100 = "H100"
	a100 = "A100"
	a30  = "A30"
	v100 = "V100"
)

var nodes = []string{"node001", "node002", "node003", "node004", "node005"}

func TestLabeler(t *testing.T) {
	cases := []struct {
		nodenames          []string
		groups             []NodeGroup
		expectedAssignment []Assignment
	}{
		{
			[]string{},
			[]NodeGroup{},
			[]Assignment{},
		},
		{
			nodes,
			[]NodeGroup{
				{
					Model:     h100,
					GPUCount:  8,
					NodeCount: 2,
				},
				{
					Model:     a100,
					GPUCount:  8,
					NodeCount: 1,
				},
				{
					Model:     a30,
					GPUCount:  8,
					NodeCount: 1,
				},
				{
					Model:     v100,
					GPUCount:  4,
					NodeCount: 1,
				},
			},
			[]Assignment{
				{
					NodeName: nodes[0],
					Model:    h100,
					GPUCount: 8,
				},
				{
					NodeName: nodes[1],
					Model:    h100,
					GPUCount: 8,
				},
				{
					NodeName: nodes[2],
					Model:    a100,
					GPUCount: 8,
				},
				{
					NodeName: nodes[6],
					Model:    a30,
					GPUCount: 8,
				},
				{
					NodeName: nodes[7],
					Model:    v100,
					GPUCount: 4,
				},
			},
		},
	}

	for _, c := range cases {
		assignment, err := AssignGroups(c.nodenames, c.groups)
		if err != nil {
			t.Errorf("failed assigning groups. err: %v", err)
		}

		if !assignmentsAreEqual(c.expectedAssignment, assignment) {
			t.Errorf("failed assigning groups. expected %v, got %v", c.expectedAssignment, assignment)
		}
	}
}

func TestLablerErrors(t *testing.T) {
	cases := []struct {
		nodenames     []string
		groups        []NodeGroup
		expectedError error
	}{
		{
			nodes,
			[]NodeGroup{
				{
					Model:     a30,
					GPUCount:  8,
					NodeCount: 8,
				},
			},
			ErrTooManyNodes,
		},
		{
			nodes,
			[]NodeGroup{
				{
					Model:     a30,
					GPUCount:  8,
					NodeCount: 1,
				},
			},
			ErrTooFewNodes,
		},
	}

	for _, c := range cases {
		_, err := AssignGroups(c.nodenames, c.groups)
		if !errors.Is(err, c.expectedError) {
			t.Errorf("expected error %v, got %v", c.expectedError, err)
		}
	}
}
