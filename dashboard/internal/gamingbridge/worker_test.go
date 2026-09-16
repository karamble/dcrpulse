package gamingbridge

import (
	"testing"
	"time"
)

func TestFinancialWorkerSharesBridgeLifetime(t *testing.T) {
	r := newBridgeRig(t)
	select {
	case <-r.workerStarted:
	case <-time.After(dialDeadline):
		t.Fatal("financial worker did not start with gaming listener")
	}
	r.srv.Stop()
	select {
	case <-r.workerStopped:
	case <-time.After(dialDeadline):
		t.Fatal("financial worker did not stop with gaming listener")
	}
}
