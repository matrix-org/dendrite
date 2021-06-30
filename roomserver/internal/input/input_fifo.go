package input

import (
	"sync"
)

type fifoQueue struct {
	tasks  []*inputTask
	count  int
	mutex  sync.Mutex
	notifs chan struct{}
}

func newFIFOQueue() *fifoQueue {
	q := &fifoQueue{
		notifs: make(chan struct{}, 1),
	}
	return q
}

func (q *fifoQueue) push(frame *inputTask) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.tasks = append(q.tasks, frame)
	q.count++
	select {
	case q.notifs <- struct{}{}:
	default:
	}
}

// pop returns the first item of the queue, if there is one.
// The second return value will indicate if a task was returned.
// You must check this value, even after calling wait().
func (q *fifoQueue) pop() (*inputTask, bool) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.count == 0 {
		return nil, false
	}
	frame := q.tasks[0]
	q.tasks[0] = nil
	q.tasks = q.tasks[1:]
	q.count--
	if q.count == 0 {
		// Force a GC of the underlying array, since it might have
		// grown significantly if the queue was hammered for some reason
		q.tasks = nil
	}
	return frame, true
}

// wait returns a channel which can be used to detect when an
// item is waiting in the queue.
func (q *fifoQueue) wait() <-chan struct{} {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.count > 0 && len(q.notifs) == 0 {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return q.notifs
}
