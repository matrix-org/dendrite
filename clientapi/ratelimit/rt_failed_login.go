package ratelimit

import (
	"container/list"
	"sync"
	"time"
)

type rateLimit struct {
	cfg   *RtFailedLoginConfig
	times *list.List
}

type RtFailedLogin struct {
	cfg *RtFailedLoginConfig
	mtx sync.RWMutex
	rts map[string]*rateLimit
}

type RtFailedLoginConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Limit    int           `yaml:"burst"`
	Interval time.Duration `yaml:"interval"`
}

// New creates a new rate limiter for the limit and interval.
func NewRtFailedLogin(cfg *RtFailedLoginConfig) *RtFailedLogin {
	if !cfg.Enabled {
		return nil
	}
	rt := &RtFailedLogin{
		cfg: cfg,
		mtx: sync.RWMutex{},
		rts: make(map[string]*rateLimit),
	}
	go rt.clean()
	return rt
}

// CanAct is expected to be called before Act
func (r *RtFailedLogin) CanAct(key string) (ok bool, remaining time.Duration) {
	r.mtx.RLock()
	rt, ok := r.rts[key]
	if !ok {
		r.mtx.RUnlock()
		return true, 0
	}
	ok, remaining = rt.canAct()
	r.mtx.RUnlock()
	return
}

// Act can be called after CanAct returns true.
func (r *RtFailedLogin) Act(key string) {
	r.mtx.Lock()
	rt, ok := r.rts[key]
	if !ok {
		rt = &rateLimit{
			cfg:   r.cfg,
			times: list.New(),
		}
		r.rts[key] = rt
	}
	rt.act()
	r.mtx.Unlock()
}

func (r *RtFailedLogin) clean() {
	for {
		r.mtx.Lock()
		for k, v := range r.rts {
			if v.empty() {
				delete(r.rts, k)
			}
		}
		r.mtx.Unlock()
		time.Sleep(time.Hour)
	}
}

func (r *rateLimit) empty() bool {
	back := r.times.Back()
	if back == nil {
		return true
	}
	v := back.Value
	b := v.(time.Time)
	now := time.Now()
	return now.Sub(b) > r.cfg.Interval
}

func (r *rateLimit) canAct() (ok bool, remaining time.Duration) {
	now := time.Now()
	l := r.times.Len()
	if l < r.cfg.Limit {
		return true, 0
	}
	frnt := r.times.Front()
	t := frnt.Value.(time.Time)
	diff := now.Sub(t)
	if diff < r.cfg.Interval {
		return false, r.cfg.Interval - diff
	}
	return true, 0
}

func (r *rateLimit) act() {
	now := time.Now()
	l := r.times.Len()
	if l < r.cfg.Limit {
		r.times.PushBack(now)
		return
	}
	frnt := r.times.Front()
	frnt.Value = now
	r.times.MoveToBack(frnt)
}
