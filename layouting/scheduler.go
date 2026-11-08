package layouting

import (
	"sync"

	"gonetsurf/layouting/interfaces"
	"gonetsurf/layouting/model"
)

// Scheduler drives incremental reflow within a frame budget. It queues
// resource arrivals and exposes a Step method so the shell can paint partial
// results while more work remains.
type Scheduler struct {
	layout *DocumentLayout

	mutex   sync.Mutex
	pending []interfaces.ResourceKind
	dirty   bool
}

// NewScheduler returns a scheduler for a document layout.
func NewScheduler(layout *DocumentLayout) *Scheduler {
	return &Scheduler{layout: layout}
}

// EnqueueResource queues a resource generation change.
func (scheduler *Scheduler) EnqueueResource(kind interfaces.ResourceKind) {
	if scheduler == nil {
		return
	}
	scheduler.mutex.Lock()
	scheduler.pending = append(scheduler.pending, kind)
	scheduler.dirty = true
	scheduler.mutex.Unlock()
}

// Dirty reports whether layout work is queued.
func (scheduler *Scheduler) Dirty() bool {
	if scheduler == nil {
		return false
	}
	scheduler.mutex.Lock()
	defer scheduler.mutex.Unlock()
	return scheduler.dirty
}

// Step performs queued layout work. A zero budget runs to completion. The
// returned result reports whether the frame is complete and what changed.
func (scheduler *Scheduler) Step(budget int) (interfaces.Result, *model.Box) {
	if scheduler == nil || scheduler.layout == nil {
		return interfaces.Result{Complete: true}, nil
	}
	scheduler.mutex.Lock()
	kinds := scheduler.pending
	scheduler.pending = nil
	scheduler.dirty = false
	scheduler.mutex.Unlock()
	for _, kind := range kinds {
		scheduler.layout.MarkResource(kind)
	}
	root := scheduler.layout.Reflow()
	return interfaces.Result{
		Complete: scheduler.layout.Error() == nil && !scheduler.layout.geometryDirty,
		Damage:   scheduler.layout.Damage(),
		Pending:  scheduler.layout.Pending(),
	}, root
}

// MarkDirty forces the next Step to reflow even without a queued resource.
func (scheduler *Scheduler) MarkDirty() {
	if scheduler == nil {
		return
	}
	scheduler.mutex.Lock()
	scheduler.dirty = true
	scheduler.mutex.Unlock()
}
