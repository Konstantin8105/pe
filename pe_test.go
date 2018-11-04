package main

import (
	"fmt"
	"testing"
)

func TestEditor(t *testing.T) {
	var ch chan bool
	go func() {
		run()
		ch <- true
	}()

	counter := 0
	ERK = func() int {
		for i, ch := range "hello\x13www\n\x11" {
			if i == counter {
				counter++
				return int(ch)
			}
		}
		panic("Not all")
	}
	fmt.Println("Done : ", <-ch)
}
