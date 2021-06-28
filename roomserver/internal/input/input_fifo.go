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
