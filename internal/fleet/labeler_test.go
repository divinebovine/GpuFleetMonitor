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
			[]string{"node001", "node002", "node003", "node004", "node005"},
			[]NodeGroup{
				{
					Model:     "H100",
					GPUCount:  8,
					NodeCount: 2,
				},
				{
					Model:     "A100",
					GPUCount:  8,
					NodeCount: 1,
				},
				{
					Model:     "A30",
					GPUCount:  8,
					NodeCount: 1,
				},
				{
					Model:     "V100",
					GPUCount:  4,
					NodeCount: 1,
				},
			},
			[]Assignment{
				{
					NodeName: "node001",
					Model:    "H100",
					GPUCount: 8,
				},
				{
					NodeName: "node002",
					Model:    "H100",
					GPUCount: 8,
				},
				{
					NodeName: "node003",
					Model:    "A100",
					GPUCount: 8,
				},
				{
					NodeName: "node004",
					Model:    "A30",
					GPUCount: 8,
				},
				{
					NodeName: "node005",
					Model:    "V100",
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
			[]string{"node001", "node002", "node003", "node004", "node005"},
			[]NodeGroup{
				{
					Model:     "A30",
					GPUCount:  8,
					NodeCount: 8,
				},
			},
			ErrTooManyNodes,
		},
		{
			[]string{"node001", "node002", "node003"},
			[]NodeGroup{
				{
					Model:     "A30",
					GPUCount:  8,
					NodeCount: 1,
				},
				{
					Model:     "H100",
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
