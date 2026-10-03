package build

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/klppl/kvist/internal/protocol"
)

func TestQueueCoalescesPendingBuilds(t *testing.T) {
	release := make(chan struct{})
	var mu sync.Mutex
	var built []string
	q := NewQueue(BuilderFunc(func(ctx context.Context, site, rev, id string) ([]protocol.Warning, error) {
		<-release
		mu.Lock()
		built = append(built, rev)
		mu.Unlock()
		return nil, nil
	}), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer q.Close()

	b1 := q.Enqueue("s", "r000001")
	// Wait until b1 is running so the next ones queue behind it.
	for {
		st, _ := q.Status("s", b1.ID)
		if st.State == protocol.BuildRunning {
			break
		}
		time.Sleep(time.Millisecond)
	}
	b2 := q.Enqueue("s", "r000002")
	b3 := q.Enqueue("s", "r000003")
	if again := q.Enqueue("s", "r000003"); again.ID != b3.ID {
		t.Errorf("same revision enqueued twice: %s vs %s", again.ID, b3.ID)
	}
	close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s2, _ := q.Wait(ctx, "s", b2.ID)
	if s2.State != protocol.BuildSuperseded || s2.SupersededBy != b3.ID {
		t.Errorf("b2 = %+v", s2)
	}
	s3, _ := q.Wait(ctx, "s", b3.ID)
	if s3.State != protocol.BuildSucceeded {
		t.Errorf("b3 = %+v", s3)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(built) != 2 || built[0] != "r000001" || built[1] != "r000003" {
		t.Errorf("built = %v", built)
	}
	q.EnqueueIfNew("s", "r000003")
	if st, _ := q.Status("s", b3.ID); st.State != protocol.BuildSucceeded {
		t.Error("EnqueueIfNew rebuilt a known revision")
	}
}
