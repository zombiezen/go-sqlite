package sqlite

import (
	"testing"
)

func TestIncrementor(t *testing.T) {
	start := 5
	i := NewIncrementor(start)
	if i == nil {
		t.Fatal("Incrementor returned nil")
	}
	if i() != start {
		t.Fatalf("first call did not start at %v", start)
	}
	for j := 1; j < 10; j++ {
		if i() != start+j {
			t.Fatalf("%v call did not return %v+%v", j, start, j)
		}
	}

	b := BindIncrementor()
	if b() != 1 {
		t.Fatal("BindIncrementor does not start at 1")
	}

	c := ColumnIncrementor()
	if c() != 0 {
		t.Fatal("ColumnIncrementor does not start at 0")
	}
}
