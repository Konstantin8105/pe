package main

import (
	"os"
	"testing"
)

type Mock struct {
	pos  int
	line []int
}

func (m *Mock) editorReadKey() int {
	defer func() {
		m.pos++
	}()
	return m.line[m.pos]
}

func (Mock) getWindowSize() (rows, cols int, err error) {
	return 100, 100, nil
}

func TestEditor(t *testing.T) {
	var m Mock
	m.line = []int{78, 101, 119, 32, 102, 105, 108, 101, 32, 13, 105, 115, 32, 99, 114, 101, 97, 116, 101, 13, 9, 78, 101, 119, 32, 100, 97, 116, 97, 13, 19, 46, 47, 116, 101, 115, 116, 100, 97, 116, 97, 47, 102, 105, 108, 101, 46, 116, 120, 116, 13, 17}

	term = &m

	f, err := os.Create("./testdata/fout")
	if err != nil {
		t.Fatal(err)
	}

	termOut = f
	err = run()
	if err != nil {
		t.Fatal(err)
	}
}
