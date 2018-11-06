package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
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
	for prefix := 0; ; prefix++ {
		keys := fmt.Sprintf("./testdata/%d.keys", prefix)
		text := fmt.Sprintf("./testdata/%d.file", prefix)
		if _, err := os.Stat(keys); os.IsNotExist(err) {
			break
		}
		if _, err := os.Stat(text); os.IsNotExist(err) {
			break
		}
		t.Run(strconv.Itoa(prefix), func(t *testing.T) {
			// parse keys
			var m Mock
			term = &m
			bs, err := ioutil.ReadFile(keys)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(bs), "\n")
			for _, l := range lines {
				if l == "" {
					continue
				}
				val, err := strconv.Atoi(strings.TrimSpace(l))
				if err != nil {
					t.Fatal(err)
				}
				m.line = append(m.line, val)
			}
			// create temp file
			out, err := ioutil.TempFile("", "")
			if err != nil {
				t.Fatal(err)
			}
			E.filename = out.Name()
			out.Close()

			// run editor with keys
			f, err := ioutil.TempFile("", "")
			if err != nil {
				t.Fatal(err)
			}
			termOut = f
			err = run()
			if err != nil {
				t.Fatal(err)
			}
			f.Close()

			// compare files content
			{
				b1, err := ioutil.ReadFile(text)
				if err != nil {
					t.Fatal(err)
				}
				b2, err := ioutil.ReadFile(E.filename)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(b1, b2) {
					t.Fatalf("is not same")
				}
			}
		})
	}
}
