package sync

import (
	"sync"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/setup/config"
)

type dummyPublisher struct {
	count int
}

func (d *dummyPublisher) SendPresence(userID, presence string) error {
	d.count++
	return nil
}

func TestRequestPool_updatePresence(t *testing.T) {
	type args struct {
		presence string
		userID   string
		sleep    time.Duration
	}
	publisher := &dummyPublisher{}
	syncMap := sync.Map{}

	tests := []struct {
		name         string
		args         args
		wantIncrease bool
	}{
		{
			name:         "new presence is published",
			wantIncrease: true,
			args: args{
				userID: "dummy",
			},
		},
		{
			name: "presence not published, no change",
			args: args{
				userID: "dummy",
			},
		},
		{
			name:         "new presence is published dummy2",
			wantIncrease: true,
			args: args{
				userID:   "dummy2",
				presence: "online",
			},
		},
		{
			name:         "different presence is published dummy2",
			wantIncrease: true,
			args: args{
				userID:   "dummy2",
				presence: "unavailable",
			},
		},
		{
			name: "same presence is not published dummy2",
			args: args{
				userID:   "dummy2",
				presence: "unavailable",
				sleep:    time.Millisecond * 150,
			},
		},
		{
			name:         "same presence is published after being deleted",
			wantIncrease: true,
			args: args{
				userID:   "dummy2",
				presence: "unavailable",
			},
		},
	}
	rp := &RequestPool{
		presence: &syncMap,
		producer: publisher,
		cfg: &config.SyncAPI{
			Matrix: &config.Global{
				JetStream: config.JetStream{
					TopicPrefix: "Dendrite",
				},
				DisablePresence: false,
			},
		},
	}
	go rp.cleanPresence(time.Millisecond * 50)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeCount := publisher.count
			rp.updatePresence(tt.args.presence, tt.args.userID)
			if tt.wantIncrease && publisher.count <= beforeCount {
				t.Fatalf("expected count to increase: %d <= %d", publisher.count, beforeCount)
			}
			time.Sleep(tt.args.sleep)
		})
	}
}
