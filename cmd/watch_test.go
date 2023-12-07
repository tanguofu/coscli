package cmd

import (
	"testing"
	"time"
)

func TestWatch(t *testing.T) {
	lastSynced := make(map[string]time.Time)

	a := "a"

	lastSynced[a] = time.Now()

	if !lastSynced["none"].Before(lastSynced[a]) {
		t.Errorf("expect none Before true")
	}

	if lastSynced[a].Before(lastSynced[a]) {
		t.Errorf("astSynced[a].Before(lastSynced[a]) false")
	}
}

func TestSelect(t *testing.T) {
	var a []int

	for i := 0; i < 3; i++ {

		select {
		case <-time.Tick(time.Second):
			a = append(a, i)
		}
	}

	if len(a) != 3 {
		t.Errorf("expect  a is 3 but :%d", len(a))
	}
}
