package timer

import (
	"container/heap"
	"fmt"
	"github.com/jageros/evq"
	"math"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	MinuteSecond = 60                // 一分钟多少秒
	HourSecond   = MinuteSecond * 60 // 一小时多少秒
	DaySeconds   = HourSecond * 24   // 一天多少秒
	WeekSecond   = DaySeconds * 7    // 一个星期多少秒
	MonthSecond  = DaySeconds * 30   // 30天多少秒
	YearSecond   = DaySeconds * 365  // 365天多少秒
)

const (
	TimeFormat = "2006-01-02 15:04:05"
	DateFormat = "2006-01-02"
)

var (
	nextAddSeq uint64 = 1
	tHeap      timerHeap
	startOnce  sync.Once
	tLock      sync.Mutex
)

var timeBaseUnix = time.Date(2020, 1, 1, 0, 0, 0, 0, time.Now().Location()).Unix()

// 设置初始日期， 用于计算从该日期起，到下一个日期的天数
func SetBaseDate(year, month, day int) {
	timeBaseUnix = time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Now().Location()).Unix()
}

type CallbackFunc func()

type Timer struct {
	fireTime time.Time
	interval time.Duration
	callback CallbackFunc
	repeat   bool
	addseq   uint64
}

func (t *Timer) GetRemainTime() time.Duration {
	now := time.Now()
	if now.Before(t.fireTime) {
		return t.fireTime.Sub(now)
	} else {
		return 0
	}
}

func (t *Timer) Cancel() {
	t.callback = nil
}

func (t *Timer) IsActive() bool {
	return t.callback != nil
}

type timerHeap struct {
	timers []*Timer
}

func (h *timerHeap) Len() int {
	return len(h.timers)
}

func (h *timerHeap) Less(i, j int) bool {
	t1, t2 := h.timers[i].fireTime, h.timers[j].fireTime
	if t1.Before(t2) {
		return true
	}

	if t1.After(t2) {
		return false
	}

	return h.timers[i].addseq < h.timers[j].addseq
}

func (h *timerHeap) Swap(i, j int) {
	var tmp *Timer
	tmp = h.timers[i]
	h.timers[i] = h.timers[j]
	h.timers[j] = tmp
}

func (h *timerHeap) Push(x interface{}) {
	h.timers = append(h.timers, x.(*Timer))
}

func (h *timerHeap) Pop() (ret interface{}) {
	l := len(h.timers)
	h.timers, ret = h.timers[:l-1], h.timers[l-1]
	return
}

func AfterFunc(d time.Duration, callback CallbackFunc) *Timer {
	t := &Timer{
		fireTime: time.Now().Add(d),
		interval: d,
		callback: callback,
		repeat:   false,
	}

	tLock.Lock()
	t.addseq = nextAddSeq
	nextAddSeq += 1

	heap.Push(&tHeap, t)
	tLock.Unlock()
	return t
}

func AddTicker(d time.Duration, callback CallbackFunc) *Timer {
	t := &Timer{
		fireTime: time.Now().Add(d),
		interval: d,
		callback: callback,
		repeat:   true,
	}

	tLock.Lock()
	t.addseq = nextAddSeq
	nextAddSeq += 1

	heap.Push(&tHeap, t)
	tLock.Unlock()
	return t
}

func TimeDelta(hour, minute, sec int) time.Duration {
	now := time.Now()
	nextTime := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, sec, 0, now.Location())
	if !now.Before(nextTime) {
		nextTime = nextTime.Add(86400 * time.Second)
	}
	return nextTime.Sub(now)
}

func TimeHourDelta(minute, sec int) time.Duration {
	now := time.Now()
	nextTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), minute, sec, 0, now.Location())
	if !now.Before(nextTime) {
		nextTime = nextTime.Add(3600 * time.Second)
	}
	return nextTime.Sub(now)
}

func TimePreDelta(hour, minute, sec int) (time.Duration, int64) {
	now := time.Now()
	preTime := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, sec, 0, now.Location())
	if !now.After(preTime) {
		preTime = preTime.Add(-86400 * time.Second)
	}
	return now.Sub(preTime), preTime.Unix()
}

func RunEveryDay(hour, minute, sec int, callback CallbackFunc) *Timer {
	t := &Timer{
		fireTime: time.Now().Add(TimeDelta(hour, minute, sec)),
		interval: 86400 * time.Second,
		callback: callback,
		repeat:   true,
	}

	tLock.Lock()
	t.addseq = nextAddSeq
	nextAddSeq += 1

	heap.Push(&tHeap, t)
	tLock.Unlock()
	return t
}

func RunEveryHour(minute, sec int, callback CallbackFunc) *Timer {
	t := &Timer{
		fireTime: time.Now().Add(TimeHourDelta(minute, sec)),
		interval: 3600 * time.Second,
		callback: callback,
		repeat:   true,
	}

	tLock.Lock()
	t.addseq = nextAddSeq
	nextAddSeq += 1

	heap.Push(&tHeap, t)
	tLock.Unlock()
	return t
}

// 默认：2020年01月01日为第一天
func GetDayNo(args ...int64) int {
	var t int64
	if len(args) > 0 {
		t = args[0]
	} else {
		t = time.Now().Unix()
	}
	return int((t-timeBaseUnix)/86400 + 1)
}

func GetWeekNo(args ...int64) int {
	var t int64
	if len(args) > 0 {
		t = args[0]
	} else {
		t = time.Now().Unix()
	}
	dayNo := GetDayNo(t)
	return int(math.Ceil((float64(dayNo)-3)/7)) + 1
}

func StartTicks(tickInterval time.Duration) {
	startOnce.Do(func() {
		go func() {
			for {
				time.Sleep(tickInterval)
				tick()
			}
		}()
	})
}

func Stop() {
	evq.Stop()
}

func tick() {
	now := time.Now()

	tLock.Lock()
	for {
		if tHeap.Len() <= 0 {
			break
		}

		nextFireTime := tHeap.timers[0].fireTime
		if nextFireTime.After(now) {
			break
		}

		t := heap.Pop(&tHeap).(*Timer)

		callback := t.callback
		if callback == nil {
			continue
		}

		if !t.repeat {
			t.callback = nil
		}

		evq.PostEvent(evq.NewCommonEvent(evq.TIMER_EVENT, callback))

		if t.repeat {
			t.fireTime = t.fireTime.Add(t.interval)
			t.addseq = nextAddSeq
			nextAddSeq += 1
			heap.Push(&tHeap, t)
		}
	}
	tLock.Unlock()
}

func onTimer(ev evq.IEvent) {
	ev.(*evq.CommonEvent).GetData()[0].(CallbackFunc)()
}

func init() {
	heap.Init(&tHeap)
	evq.HandleEvent(evq.TIMER_EVENT, onTimer)
}
