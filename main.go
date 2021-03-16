package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	coordinateMatch = regexp.MustCompile(`([A-Z]+)([0-9]+)`)
	filesMatch      = regexp.MustCompile(`^FILES\((.*)\)$`)
)

type Grid [][]string

type Sheet struct {
	grid    Grid
	edges   map[Coordinate][]Coordinate
	files   []Coordinate
	modTime time.Time
}

func (s *Sheet) HasCycles() bool {
	seen := map[Coordinate]struct{}{}

	var hasCycles func(edges []Coordinate) bool
	hasCycles = func(edges []Coordinate) bool {
		for _, edge := range edges {
			if _, ok := seen[edge]; ok {
				return true
			}
			seen[edge] = struct{}{}
			if hasCycles(s.edges[edge]) {
				return true
			}
		}
		return false
	}

	for node, edges := range s.edges {
		if _, ok := seen[node]; ok {
			// if we've crawled this graph, skip
			continue
		}
		seen[node] = struct{}{}
		if hasCycles(edges) {
			return true
		}
	}
	return false
}

func (s *Sheet) AddEdge(a, b Coordinate) {
	s.edges[a] = append(s.edges[a], b)
}

func (s *Sheet) WriteConfig(path string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	writer := csv.NewWriter(f)
	defer writer.Flush()
	return writer.WriteAll(s.grid)
}

func (s *Sheet) cellValue(coo Coordinate) string {
	defer func() {
		_ = recover()
	}()
	return s.grid[coo[1]][coo[0]]
}

func (s *Sheet) doesCellContainModifiedFiles(coo Coordinate) bool {
	for _, match := range filesMatch.FindAllStringSubmatch(s.cellValue(coo), -1) {
		files, _ := filepath.Glob(match[1])
		for _, file := range files {
			fi, err := os.Stat(file)
			if err == nil {
				if fi.ModTime().After(s.modTime) {
					return true
				}
			}
		}
	}
	return false
}

func (s *Sheet) NewFiles() []Coordinate {
	out := []Coordinate{}
	for _, coo := range s.files {
		if s.doesCellContainModifiedFiles(coo) {
			out = append(out, coo)
		}
	}
	return out
}

func NewSheet(path string) (*Sheet, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer f.Close()
	reader := csv.NewReader(f)
	grid, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	return NewSheetFromGrid(grid, fi.ModTime()), nil
}

func NewSheetFromGrid(g Grid, modTime time.Time) *Sheet {
	s := &Sheet{
		edges:   map[Coordinate][]Coordinate{},
		grid:    g,
		modTime: modTime,
	}
	for x, row := range g {
		for y, val := range row {
			coordinates := CoordinatesInCell(val)
			for _, coo := range coordinates {
				// Add link from the source cell to the cell referencing the value
				s.AddEdge(coo, Coordinate{x, y})
			}
			if filesMatch.MatchString(strings.TrimSpace(val)) {
				s.files = append(s.files, Coordinate{x, y})
			}
		}
	}
	return s
}

func CoordinatesInCell(cell string) []Coordinate {
	out := []Coordinate{}
	os.Expand(cell, func(v string) string {
		for _, matches := range coordinateMatch.FindAllStringSubmatch(v, -1) {
			xString, yString := matches[2], matches[1]
			num, err := strconv.Atoi(xString)
			if err != nil {
				return ""
			}
			// disallow things like padding 00001
			if fmt.Sprint(num) != xString {
				return ""
			}
			x := num - 1
			if x < 0 {
				return "" // don't support $A0
			}
			out = append(out, Coordinate{x, columnNameToIndex(yString)})
		}
		return ""
	})
	return out
}

type Coordinate [2]int

func (coo Coordinate) String() string {
	return columnIndexToColumnName(coo[1]) + fmt.Sprint(coo[0]+1)
}

func columnNameToIndex(name string) int {
	number := 0
	pow := 1
	for i := len(name) - 1; i >= 0; i-- {
		c := name[i]
		number += int(c-'A'+1) * pow
		pow *= 26
	}
	return number - 1
}

func columnIndexToColumnName(num int) string {
	// https://stackoverflow.com/a/182924/1333724
	dividend := num + 1
	var columnName string
	var modulo int
	for dividend > 0 {
		modulo = (dividend - 1) % 26
		columnName = string('A'+byte(modulo)) + columnName
		dividend = (dividend - modulo) / 26
	}
	return columnName
}

func main() {
	dataLocation := "./eggshell.csv"
	sheet, err := NewSheet(dataLocation)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	newFiles := sheet.NewFiles()
	if newFiles == nil {
		return
	}
	if sheet.HasCycles() {
		fmt.Fprintln(os.Stderr, "graph has circular references")
		os.Exit(1)
	}

	for _, coo := range newFiles {

	}
	_ = sheet.WriteConfig(dataLocation)
}
