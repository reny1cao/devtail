package queue

import (
	"container/list"
	"sync"
	"time"

	"github.com/devtail/gateway/pkg/protocol"
)

type QueueItem struct {
	Message   *protocol.Message
	Timestamp time.Time
	Retries   int
}

type MessageQueue struct {
	mu              sync.RWMutex
	pending         *list.List
	inFlight        map[string]*QueueItem
	maxRetries      int
	retryTimeout    time.Duration
	maxQueueSize    int
	sequenceCounter uint64
}

func NewMessageQueue(maxQueueSize, maxRetries int, retryTimeout time.Duration) *MessageQueue {
	return &MessageQueue{
		pending:      list.New(),
		inFlight:     make(map[string]*QueueItem),
		maxRetries:   maxRetries,
		retryTimeout: retryTimeout,
		maxQueueSize: maxQueueSize,
	}
}

func (q *MessageQueue) Enqueue(msg *protocol.Message) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.pending.Len() >= q.maxQueueSize {
		oldest := q.pending.Front()
		if oldest != nil {
			q.pending.Remove(oldest)
		}
	}

	q.sequenceCounter++
	msg.SeqNum = q.sequenceCounter

	item := &QueueItem{
		Message:   msg,
		Timestamp: time.Now(),
		Retries:   0,
	}

	q.pending.PushBack(item)
	return nil
}

func (q *MessageQueue) Dequeue() *protocol.Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	elem := q.pending.Front()
	if elem == nil {
		return nil
	}

	item := elem.Value.(*QueueItem)
	q.pending.Remove(elem)
	
	q.inFlight[item.Message.ID] = item
	
	return item.Message
}

func (q *MessageQueue) Ack(messageID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.inFlight, messageID)
}

func (q *MessageQueue) CheckRetries() []*protocol.Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	var toRetry []*protocol.Message
	now := time.Now()

	for id, item := range q.inFlight {
		if now.Sub(item.Timestamp) > q.retryTimeout {
			if item.Retries < q.maxRetries {
				item.Retries++
				item.Timestamp = now
				toRetry = append(toRetry, item.Message)
			} else {
				delete(q.inFlight, id)
			}
		}
	}

	return toRetry
}

func (q *MessageQueue) GetPendingCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.pending.Len()
}

func (q *MessageQueue) GetInFlightCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.inFlight)
}

func (q *MessageQueue) GetMessagesAfter(seqNum uint64) []*protocol.Message {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var messages []*protocol.Message
	
	for e := q.pending.Front(); e != nil; e = e.Next() {
		item := e.Value.(*QueueItem)
		if item.Message.SeqNum > seqNum {
			messages = append(messages, item.Message)
		}
	}

	for _, item := range q.inFlight {
		if item.Message.SeqNum > seqNum {
			messages = append(messages, item.Message)
		}
	}

	return messages
}