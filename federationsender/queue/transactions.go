package queue

import (
	"fmt"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/federationsender/statistics"
	"github.com/matrix-org/dendrite/federationsender/storage/shared"
)

const (
	maxTransactionsInMemory = 4
	maxPDUsPerTransaction   = 50
	maxEDUsPerTransaction   = 50
)

type queuedTransaction struct {
	id          gomatrixserverlib.TransactionID
	pdus        []*gomatrixserverlib.HeaderedEvent
	edus        []*gomatrixserverlib.EDU
	pduReceipts []*shared.Receipt
	eduReceipts []*shared.Receipt
}

type queuedTransactions struct {
	statistics *statistics.ServerStatistics
	queue      []*queuedTransaction
}

func (q *queuedTransactions) createNew(transactionID gomatrixserverlib.TransactionID) bool {
	now := gomatrixserverlib.AsTimestamp(time.Now())
	if len(q.queue) == maxTransactionsInMemory {
		return false
	}
	if transactionID == "" {
		transactionID = gomatrixserverlib.TransactionID(fmt.Sprintf("%d-%d", now, q.statistics.SuccessCount()))
	}
	q.queue = append(q.queue, &queuedTransaction{
		id: transactionID,
	})
	return true
}

func (q *queuedTransactions) getNewestForPDU() *queuedTransaction {
	if len(q.queue) == 0 || len(q.queue[len(q.queue)-1].pdus) == maxPDUsPerTransaction {
		if len(q.queue) == maxTransactionsInMemory {
			return nil
		}
		if !q.createNew("") {
			return nil
		}
	}
	return q.queue[len(q.queue)-1]
}

func (q *queuedTransactions) getNewestForEDU() *queuedTransaction {
	if len(q.queue) == 0 || len(q.queue[len(q.queue)-1].edus) == maxEDUsPerTransaction {
		if len(q.queue) == maxTransactionsInMemory {
			return nil
		}
		if !q.createNew("") {
			return nil
		}
	}
	return q.queue[len(q.queue)-1]
}

func (q *queuedTransactions) queuePDUs(receipt *shared.Receipt, pdu ...*gomatrixserverlib.HeaderedEvent) gomatrixserverlib.TransactionID {
	last := q.getNewestForPDU()
	if last == nil {
		return ""
	}
	last.pdus = append(last.pdus, pdu...)
	if receipt != nil {
		last.pduReceipts = append(last.pduReceipts, receipt)
	}
	return last.id
}

func (q *queuedTransactions) queueEDUs(receipt *shared.Receipt, edu ...*gomatrixserverlib.EDU) gomatrixserverlib.TransactionID {
	last := q.getNewestForEDU()
	if last == nil {
		return ""
	}
	last.edus = append(last.edus, edu...)
	if receipt != nil {
		last.eduReceipts = append(last.eduReceipts, receipt)
	}
	return last.id
}
