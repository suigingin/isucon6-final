package scenario

import (
	"time"

	"github.com/catatsuy/isucon6-final/bench/score"
	"github.com/catatsuy/isucon6-final/bench/seed"
	"github.com/catatsuy/isucon6-final/bench/session"
)

const (
	initialWatcherNum             = 5
	watcherIncreaseInterval       = 5
	StrokeReceiveScore      int64 = 1
)

// 一人がroomを作る→大勢がそのroomをwatchする
func Matsuri(origins []string, timeout int) {
	s := session.New(randomOrigin(origins))
	defer s.Bye()

	token, ok := fetchCSRFToken(s, "/")
	if !ok {
		return
	}

	roomID, ok := makeRoom(s, token)
	if !ok {
		return
	}

	strokes := seed.GetStrokes("isu")

	postTimes := make(map[int64]time.Time)

	start := time.Now()

	go func() {
		// 1秒おきにstrokeをPOSTする
		for {
			for _, stroke := range strokes {
				postTime := time.Now()

				strokeID, ok := drawStroke(s, token, roomID, seed.FluctuateStroke(stroke))
				if ok {
					postTimes[strokeID] = postTime
				}
				time.Sleep(1 * time.Second)
				if time.Now().Sub(start).Seconds() > float64(timeout) {
					return
				}
			}
		}
	}()

	watchers := make([]*RoomWatcher, 0)

	for {
		// watcherIncreaseInterval秒おきに、まだ退室していないwatcherの数と同数の人数が入室する

		n := 0
		for _, w := range watchers {
			if len(w.EndCh) == 0 {
				n++
			}
		}
		if n == 0 { // ゼロならinitialWatcherNum人が入室する（特に初回）
			n = initialWatcherNum
		}
		for i := 0; i < n; i++ {
			watchers = append(watchers, NewRoomWatcher(randomOrigin(origins), roomID))
		}

		time.Sleep(time.Duration(watcherIncreaseInterval) * time.Second)
		if time.Now().Sub(start).Seconds() > float64(timeout-watcherIncreaseInterval) {
			break
		}
	}

	// ここまでで最大 initialWatcherNum * 2 ^ ((timeout - watcherIncreaseInterval) / watcherIncreaseInterval) 人が入室してるはず
	// 例えば initialWatcherNum=10, timeout=55, watcherIncreaseInterval=5 なら 10 * 2 ^ ((55-5)/5) = 10240 人

	//fmt.Println("stop")
	for _, w := range watchers {
		w.Leave()
	}
	//fmt.Println("wait")
	for _, w := range watchers {
		<-w.EndCh
	}
	//fmt.Println("done")

	for _, w := range watchers {
		for _, strokeLog := range w.StrokeLogs {
			postTime := postTimes[strokeLog.Stroke.ID]
			timeTaken := strokeLog.ReceivedTime.Sub(postTime).Seconds()

			if timeTaken < 1 { // TODO: この時間は要調整
				score.Increment(StrokeReceiveScore * 2)
			} else if timeTaken < 3 {
				score.Increment(StrokeReceiveScore)
			}
		}
	}
}
