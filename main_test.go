package main

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestCoordinatesInCell(t *testing.T) {
	tests := []struct {
		cell string
		want []Coordinate
	}{
		{
			cell: "echo $A1",
			want: []Coordinate{{0, 0}},
		},
		{
			cell: "echo $F010",
			want: []Coordinate{},
		},
		{
			cell: "curl $A1 $B3",
			want: []Coordinate{{0, 0}, {2, 1}},
		},
		{
			cell: "curl $A1 $AB3 $DKK7",
			want: []Coordinate{{0, 0}, {2, 27}, {6, 3000}},
		},
		{
			cell: "$A0",
			want: []Coordinate{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.cell, func(t *testing.T) {
			if got := CoordinatesInCell(tt.cell); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CoordinatesInCell() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_columnIndexToColumnName(t *testing.T) {
	tests := []struct {
		num  int
		want string
	}{
		{num: 0, want: "A"},
		{num: 27, want: "AB"},
		{num: 156, want: "FA"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.num), func(t *testing.T) {
			if got := columnIndexToColumnName(tt.num); got != tt.want {
				t.Errorf("columnIndexToColumnName() = %v, want %v", got, tt.want)
			}
		})
	}
	// whatever, test em all
	for i := 0; i < 1e6; i++ {
		if v := columnNameToIndex(columnIndexToColumnName(i)); v != i {
			t.Errorf("columnIndexToColumnName() = %v, want %v", v, i)
		}
	}
}

func TestSheet_HasCycles(t *testing.T) {
	tests := []struct {
		name string
		grid Grid
		want bool
	}{
		{
			name: "all good",
			grid: Grid{
				{"FILES(main.go)"},
				{"FILES(*.go)"},
				{"echo $A1"},
				{"echo $A3"},
			},
			want: false,
		}, {
			name: "trivial circle",
			grid: Grid{
				{"echo $A2"},
				{"echo $A1"},
			},
			want: true,
		}, {
			name: "more complicated cycle, two graphs",
			grid: Grid{
				{"echo $A3"},
				{"echo $A4"},
				{"echo $A5"},
				{"echo $A6"},
				{"echo $A1"},
				{"echo $A2"},
			},
			want: true,
		}, {
			name: "more complicated cycle, two graphs 2",
			grid: Grid{
				{"echo $A2", "echo $B3"},
				{"echo $A3", "echo $B2"},
				{"echo $A1", "echo $B1"},
			},
			want: true,
		}, {
			name: "two disconnected acyclic graphs",
			grid: Grid{
				{"echo $A2", "echo $B2"},
				{"echo $A3", "echo $B3"},
				{"echo 1", "echo 1"},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSheetFromGrid(tt.grid, time.Time{})
			if got := s.graph.Validate() != nil; got != tt.want {
				t.Errorf("Sheet.HasCycles() = %v, want %v", got, tt.want)
			}
		})
	}
}
