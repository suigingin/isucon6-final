package scenario

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/catatsuy/isucon6-final/bench/action"
	"github.com/catatsuy/isucon6-final/bench/fails"
	"github.com/catatsuy/isucon6-final/bench/seed"
	"github.com/catatsuy/isucon6-final/bench/session"
)

// 部屋を作って線を描くとトップページに出てくる & 線がSVGに反映される
func StrokeReflectedToTop(origins []string) {
	s1 := session.New(randomOrigin(origins))
	s2 := session.New(randomOrigin(origins))
	defer s1.Bye()
	defer s2.Bye()

	token, ok := fetchCSRFToken(s1, "/")
	if !ok {
		return
	}

	t1 := time.Now()

	room, ok := makeRoom(s1, token)
	if !ok {
		fails.Critical("部屋の作成に失敗しました", nil)
		return
	}

	t2 := time.Now()
	if room.CreatedAt.After(t2) || room.CreatedAt.Before(t1) {
		fails.Critical("作成した部屋のcreated_atが正しくありません",
			fmt.Errorf("should be %s < %s < %s", t1.Format("2006-01-02-15:04:05.000"), room.CreatedAt.Format("2006-01-02-15:04:05.000"), t2.Format("2006-01-02-15:04:05.000")))
	}

	seedStrokes := seed.GetStrokes("ramen")
	seedStroke := seed.FluctuateStroke(seedStrokes[0])
	stroke, ok := drawStroke(s1, token, room.ID, seedStroke)
	if !ok {
		fails.Critical("線の投稿に失敗しました", nil)
		return
	}

	t3 := time.Now()
	if stroke.CreatedAt.After(t3) || stroke.CreatedAt.Before(t2) {
		fails.Critical("作成した部屋のcreated_atが正しくありません",
			fmt.Errorf("should be %s < %s < %s", t2.Format("2006-01-02-15:04:05.000"), stroke.CreatedAt.Format("2006-01-02-15:04:05.000"), t3.Format("2006-01-02-15:04:05.000")))
	}

	// 描いた直後にトップページに表示される
	ok = action.Get(s2, "/", action.OK(func(body io.Reader, l *fails.Logger) bool {
		doc, ok := makeDocument(body, l)
		if !ok {
			return false
		}
		imageUrls := extractImages(doc)

		found := false
		for _, img := range imageUrls {
			if img == "/img/"+strconv.FormatInt(room.ID, 10) {
				found = true
			}
		}
		if !found {
			l.Critical("投稿が反映されていません", nil)
			return false
		}
		return true
	}))
	if !ok {
		return
	}

	// SVGに反映される
	for _, seedStroke := range seedStrokes[1:] {
		stroke2 := seed.FluctuateStroke(seedStroke)
		stroke, ok := drawStroke(s1, token, room.ID, stroke2)
		if !ok {
			fails.Critical("線の投稿に失敗しました", nil)
			break
		}

		ok = checkStrokeReflectedToSVG(s2, room.ID, stroke.ID, stroke2)
		if !ok {
			break
		}
	}
}

// 線の描かれてない部屋はトップページに並ばない
func RoomWithoutStrokeNotShownAtTop(origins []string) {
	s1 := session.New(randomOrigin(origins))
	s2 := session.New(randomOrigin(origins))
	defer s1.Bye()
	defer s2.Bye()

	token, ok := fetchCSRFToken(s1, "/")
	if !ok {
		return
	}

	room, ok := makeRoom(s1, token)
	if !ok {
		fails.Critical("部屋の作成に失敗しました", nil)
		return
	}

	_ = action.Get(s2, "/", action.OK(func(body io.Reader, l *fails.Logger) bool {
		doc, ok := makeDocument(body, l)
		if !ok {
			return false
		}
		imageUrls := extractImages(doc)

		for _, img := range imageUrls {
			if img == "/img/"+strconv.FormatInt(room.ID, 10) {
				l.Critical("まだ線の無い部屋が表示されています", nil)
				return false
			}
		}
		return true
	}))
}

// ページ内のCSRFトークンが毎回変わっている
func CSRFTokenRefreshed(origins []string) {
	s1 := session.New(randomOrigin(origins))
	s2 := session.New(randomOrigin(origins))
	defer s1.Bye()
	defer s2.Bye()

	token1, ok := fetchCSRFToken(s1, "/")
	if !ok {
		return
	}

	token2, ok := fetchCSRFToken(s2, "/")
	if !ok {
		return
	}

	if token1 == token2 {
		fails.Critical("csrf_tokenが使いまわされています", nil)
	}
}

// 他人の作った部屋に最初の線を描けない
func CantDrawFirstStrokeOnSomeoneElsesRoom(origins []string) {
	s1 := session.New(randomOrigin(origins))
	s2 := session.New(randomOrigin(origins))
	defer s1.Bye()
	defer s2.Bye()

	token1, ok := fetchCSRFToken(s1, "/")
	if !ok {
		return
	}

	room, ok := makeRoom(s1, token1)
	if !ok {
		fails.Critical("部屋の作成に失敗しました", nil)
		return
	}

	token2, ok := fetchCSRFToken(s2, "/")
	if !ok {
		return
	}

	strokes := seed.GetStrokes("star")
	stroke := seed.FluctuateStroke(strokes[0])

	postBody, _ := json.Marshal(struct {
		RoomID int64 `json:"room_id"`
		seed.Stroke
	}{
		RoomID: room.ID,
		Stroke: stroke,
	})

	headers := map[string]string{
		"Content-Type": "application/json",
		"x-csrf-token": token2,
	}

	u := "/api/strokes/rooms/" + strconv.FormatInt(room.ID, 10)
	ok = action.Post(s2, u, postBody, headers, action.BadRequest(func(body io.Reader, l *fails.Logger) bool {
		// JSONも検証する？
		return true
	}))
	if !ok {
		fails.Critical("他人の作成した部屋に1画目を描くことができました", nil)
	}
}

// トップページの内容が正しいかをチェック
func TopPageContent(origins []string) {
	s := session.New(randomOrigin(origins))
	defer s.Bye()

	_ = action.Get(s, "/", action.OK(func(body io.Reader, l *fails.Logger) bool {
		doc, ok := makeDocument(body, l)
		if !ok {
			return false
		}
		images := extractImages(doc)
		if len(images) < 100 {
			l.Critical("画像の枚数が少なすぎます", nil)
			return false
		}

		reactidNum := doc.Find("[data-reactid]").Length()
		expected := 1525
		if reactidNum != expected {
			l.Critical("トップページの内容が正しくありません",
				fmt.Errorf("data-reactidの数が一致しません (expected %d, actual %d)", expected, reactidNum))
			return false
		}
		return true
	}))
}

// 静的ファイルが正しいかをチェック
func CheckStaticFiles(origins []string) {
	s := session.New(randomOrigin(origins))
	defer s.Bye()

	ok := loadStaticFiles(s, true /*checkHash*/)
	if !ok {
		fails.Critical("静的ファイルが正しくありません", nil)
	}
}

// APIとHTMLの整合性が取れているかをチェック
func APIAndHTMLMustBeConsistent(origins []string) {
	s := session.New(randomOrigin(origins))
	defer s.Bye()

	rooms, ok := getRoomsAPI(s)
	if !ok {
		fails.Critical("部屋一覧APIの取得に失敗しました", nil)
		return
	}

	ok = compareToTopHTML(s, rooms)
	if !ok {
		return
	}

	room := rooms[rand.Intn(50)+50] // 後ろの方から

	room2, ok := getRoomAPI(s, room.ID)
	if !ok {
		fails.Critical("部屋APIの取得に失敗しました", nil)
		return
	}
	ok = compareToRoomHTML(s, room2.ID, room2.Strokes)
}

func compareToTopHTML(s *session.Session, rooms []Room) bool {
	roomStrokeCounts := make(map[int64]int)

	ok := action.Get(s, "/", action.OK(func(body io.Reader, l *fails.Logger) bool {
		doc, ok := makeDocument(body, l)
		if !ok {
			return false
		}
		doc.Find(".room").Each(func(i int, sel *goquery.Selection) {
			idStr, ok := sel.Attr("id")
			if !ok {
				fails.Critical("roomのidがありません", nil)
				return
			}
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				fails.Critical("roomのidが数字ではありません", err)
				return
			}
			strokeCountStr := sel.Find(".stroke_count").Text()
			if strokeCountStr == "" {
				fails.Critical("stroke_countがありません", err)
				return
			}
			strokeCount, err := strconv.Atoi(strokeCountStr)
			if err != nil {
				fails.Critical("stroke_countが数字ではありません", err)
				return
			}
			roomStrokeCounts[id] = strokeCount
		})
		if len(roomStrokeCounts) != 100 {
			fails.Critical("部屋の数が100件になっていません: "+strconv.Itoa(len(roomStrokeCounts)), nil)
			return false
		}

		return true
	}))
	if !ok {
		return false
	}

	bad := 0
	for _, room := range rooms {
		if roomStrokeCount, ok := roomStrokeCounts[room.ID]; ok {
			if roomStrokeCount < room.StrokeCount {
				fails.Critical("APIとHTMLの差分が大きすぎます", nil)
				return false
			}
		} else {
			bad++
		}
	}
	if bad > 90 {
		fails.Critical("APIとHTMLの差分が大きすぎます", nil)
		return false
	}
	return true
}

func compareToRoomHTML(s *session.Session, roomID int64, strokes []Stroke) bool {
	roomURL := "/rooms/" + strconv.FormatInt(roomID, 10)
	ok := action.Get(s, roomURL, action.OK(func(body io.Reader, l *fails.Logger) bool {
		doc, ok := makeDocument(body, l)
		if !ok {
			return false
		}
		if doc.Find("polyline[id]").Length() < len(strokes) {
			fails.Critical("APIとHTMLの差分が大きすぎます", nil)
		}
		return true
	}))
	if !ok {
		return false
	}
	return true
}
