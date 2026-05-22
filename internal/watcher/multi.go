package watcher

import "context"

// MultiWatcher fans the output of multiple Watchers into one Snapshot channel.
type MultiWatcher struct {
	Watchers []Watcher
}

func (m *MultiWatcher) Run(ctx context.Context, out chan<- Snapshot) {
	if len(m.Watchers) == 0 {
		<-ctx.Done()
		return
	}
	merged := make(chan Snapshot, len(m.Watchers)*4)
	for _, w := range m.Watchers {
		go w.Run(ctx, merged)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case snap := <-merged:
			send(ctx, out, snap)
		}
	}
}
