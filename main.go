package main

import (
	"bytes"

	"encoding/csv"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/maxmcd/dag"
)

var (
	coordinateMatch = regexp.MustCompile(`([A-Z]+)([0-9]+)`)
	filesMatch      = regexp.MustCompile(`^FILES\((.*)\)$`)
)

type Grid [][]string

type Sheet struct {
	grid    Grid
	graph   dag.AcyclicGraph
	files   []Coordinate
	modTime time.Time
}

func (s *Sheet) AddEdge(a, b Coordinate) {
	s.graph.Add(a)
	s.graph.Add(b)
	s.graph.Connect(dag.BasicEdge(a, b))
}

func (s *Sheet) UpdateCell(row, column int, value string) {
	fmt.Println(s.cellValue(Coordinate{row, column}))
}

func (s *Sheet) HasCycles() (err error) {
	// Look for cycles of more than 1 component
	cycles := s.graph.Cycles()
	if len(cycles) > 0 {
		for _, cycle := range cycles {
			cycleStr := make([]string, len(cycle))
			for j, vertex := range cycle {
				cycleStr[j] = dag.VertexName(vertex)
			}

			err = multierror.Append(err, fmt.Errorf(
				"Cycle: %s", strings.Join(cycleStr, ", ")))
		}
	}

	// Look for cycles to self
	for _, e := range s.graph.Edges() {
		if e.Source() == e.Target() {
			err = multierror.Append(err, fmt.Errorf(
				"Self reference: %s", dag.VertexName(e.Source())))
		}
	}
	return
}

func (s *Sheet) Subgraph(a ...Coordinate) (subgraph dag.AcyclicGraph) {
	if len(a) == 0 {
		return subgraph
	}
	queue := a
	for {
		node := queue[0]
		for _, edge := range s.graph.EdgesTo(node) {
			subgraph.Add(edge.Source())
			subgraph.Add(edge.Target())
			subgraph.Connect(edge)
			queue = append(queue, edge.Source().(Coordinate))
		}
		queue = queue[1:]
		if len(queue) == 0 {
			break
		}
	}

	// If we have not root (which is common in a spreadsheet) we need to
	// make a fake one
	roots := graphRoots(subgraph)
	if len(roots) > 1 {
		subgraph.Add(fakeRoot)
		for _, c := range roots {
			subgraph.Add(c)
			subgraph.Connect(dag.BasicEdge(fakeRoot, c))
		}
	}
	return
}

func graphRoots(g dag.AcyclicGraph) []dag.Vertex {
	roots := make([]dag.Vertex, 0, 1)
	for _, v := range g.Vertices() {
		if g.UpEdges(v).Len() == 0 {
			roots = append(roots, v)
		}
	}
	return roots
}

func (s *Sheet) WriteConfig(path string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	writer := csv.NewWriter(f)
	defer writer.Flush()
	s.quoteEmptyStrings()
	return writer.WriteAll(s.grid)
}

// https://github.com/golang/go/issues/39119
func (s *Sheet) quoteEmptyStrings() {
	for i, row := range s.grid {
		for j, cell := range row {
			if cell == "" {
				s.grid[i][j] = `""`
			}
		}
	}
}

func (s *Sheet) cellValue(coo Coordinate) string {
	defer func() {
		_ = recover()
	}()
	return s.grid[coo[0]][coo[1]]
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
		grid:    g,
		modTime: modTime,
	}
	for x, row := range g {
		for y, val := range row {
			coordinates := CoordinatesInCell(val)
			for _, coo := range coordinates {
				// Add link from the source cell to the cell referencing the value
				s.AddEdge(Coordinate{x, y}, coo)
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
	Expand(cell, func(coo Coordinate) string {
		out = append(out, coo)
		return ""
	})
	return out
}

func Expand(cell string, fn func(Coordinate) string) string {
	return os.Expand(cell, func(v string) string {
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
			coo := Coordinate{x, columnNameToIndex(yString)}
			return fn(coo)
		}
		return ""
	})
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

var fakeRoot = Coordinate{-1, -1}

func run() (err error) {
	dataLocation := "./eggshell.csv"
	sheet, err := NewSheet(dataLocation)
	if err != nil {
		return err
	}

	log.Fatal(sheet.RunServer(":8080"))
	if err := sheet.HasCycles(); err != nil {
		return err
	}
	newFiles := sheet.NewFiles()
	if newFiles == nil {
		return
	}

	sheet.grid[3][0] = "echo \"$A3\""

	toRun := sheet.Subgraph(newFiles...)
	outputs := map[Coordinate]string{}
	lock := sync.Mutex{}
	errs := toRun.Walk(func(v dag.Vertex) error {
		coo := v.(Coordinate)
		if coo == fakeRoot {
			return nil
		}
		value := sheet.cellValue(coo)
		if filesMatch.MatchString(value) {
			files, _ := filepath.Glob(filesMatch.FindAllStringSubmatch(value, 1)[0][1])
			lock.Lock()
			outputs[coo] = strings.Join(files, " ")
			lock.Unlock()
			return nil
		}
		cmd := exec.Command("bash", "-c", value)
		var buf bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &stderr

		cmd.Env = os.Environ()

		coos := CoordinatesInCell(value)
		for _, coo := range coos {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", coo, outputs[coo]))
		}
		if err := cmd.Run(); err != nil {
			fmt.Println(stderr.String())
			return err
		}

		fmt.Println(buf.String())
		lock.Lock()
		outputs[coo] = buf.String()
		lock.Unlock()
		return nil
	})
	_ = errs
	// for _, err := range errs {
	// 	fmt.Println(err)
	// }
	// if len(errs) > 0 {
	// 	return errors.New("errors running")
	// }
	return sheet.WriteConfig(dataLocation)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
