package gamingbridge

import "testing"

func TestAGameHoldsAtMostFourStreams(t *testing.T) {
	r := newRegistry()
	var open []*liveStream
	for i := 0; i < maxStreamsPerGame; i++ {
		l := r.add("poker")
		if l == nil {
			t.Fatalf("stream %d refused", i+1)
		}
		open = append(open, l)
	}
	if r.add("poker") != nil {
		t.Fatal("a fifth stream was admitted")
	}
	if r.add("chess") == nil {
		t.Fatal("another game's stream was refused")
	}
	r.remove("poker", open[0])
	if r.add("poker") == nil {
		t.Fatal("a freed slot was not reusable")
	}
	if maxStreamsPerGame != 4 {
		t.Fatalf("cap = %d", maxStreamsPerGame)
	}
}
