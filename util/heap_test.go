package util

import (
	"testing"
	"time"
)

func TestIntHeap(t *testing.T) {
	h := NewFileChangedHeap()

	h.Update("a1", time.Now())
	time.Sleep(time.Second)
	h.Update("b2", time.Now())
	time.Sleep(time.Second)
	h.Update("c3", time.Now())
	time.Sleep(time.Second)

	h.Update("a1", time.Now())
	time.Sleep(time.Second)

	t1, a1, c1 := h.PopOlder()
	if !t1 || a1 != "b2" {
		t.Errorf("expect b2 true, but: %t, %s, %v", t1, a1, c1)
	}

	t2, a2, c2 := h.PopOlder()
	if !t2 || a1 != "c3" {
		t.Errorf("expect c3 true, but: %t, %s, %v", t2, a2, c2)
	}

	t3, a3, c3 := h.PopOlder()
	if !t3 || a3 != "a1" {
		t.Errorf("expect a1 true, but: %t, %s, %v", t3, a3, c3)
	}

	t4, a4, c4 := h.PopOlder()

	if t4 {
		t.Errorf("expect t4 false, but: %t, %s, %v", t4, a4, c4)
	}

}
