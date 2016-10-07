package scenario

import (
	"errors"
	"io"
	"math/rand"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/catatsuy/isucon6-final/bench/fails"
	"github.com/catatsuy/isucon6-final/bench/score"
	"github.com/catatsuy/isucon6-final/bench/session"
)

var (
	IndexGetScore      int64 = 1
	RoomGetScore       int64 = 1
	SVGGetScore        int64 = 1
	CreateRoomScore    int64 = 20
	CreateStrokeScore  int64 = 20
	StrokeReceiveScore int64 = 1
)

func extractImages(doc *goquery.Document) []string {
	imageUrls := []string{}

	doc.Find("img").Each(func(_ int, selection *goquery.Selection) {
		if url, ok := selection.Attr("src"); ok {
			imageUrls = append(imageUrls, url)
		}
	})

	return imageUrls
}

func extractCsrfToken(doc *goquery.Document) string {
	var token string

	doc.Find("html").Each(func(_ int, selection *goquery.Selection) {
		if t, ok := selection.Attr("data-csrf-token"); ok {
			token = t
		}
	})

	return token
}

// TODO: ステータスコード以外にもチェックしたい
func loadImages(s *session.Session, images []string) error {
	var lastErr error
	for _, image := range images {
		err := s.Get(image, func(body io.Reader, l *fails.Logger) error {
			score.Increment(SVGGetScore)
			return nil
		})
		if err != nil {
			lastErr = err
		}
	}
	return lastErr

	// TODO: 画像を並列リクエストするようにしてみたが、 connection reset by peer というエラーが出るので直列に戻した
	// もしかすると s.Transport.MaxIdleConnsPerHost ずつ処理するといけるのかも
	//errs := make(chan error, len(images))
	//for _, image := range images {
	//	go func(image string) {
	//		err := s.Get(image, func(status int, body io.Reader) error {
	//			if status != 200 {
	//				return errors.New("ステータスが200ではありません: " + strconv.Itoa(status))
	//			}
	//			score.Increment(SVGGetScore)
	//			return nil
	//		})
	//		errs <- err
	//	}(image)
	//}
	//var lastErr error
	//for i := 0; i < len(images); i++ {
	//	err := <-errs
	//	if err != nil {
	//		lastErr = err
	//	}
	//}
	//return lastErr
}

// トップページと画像に負荷をかける
func LoadIndexPage(s *session.Session) {
	var token string
	var images []string

	err := s.Get("/", func(body io.Reader, l *fails.Logger) error {
		doc, err := goquery.NewDocumentFromReader(body)
		if err != nil {
			l.Add("ページのHTMLがパースできませんでした", err)
			return err
		}

		token = extractCsrfToken(doc)

		if token == "" {
			l.Add("csrf_tokenが取得できませんでした", nil)
			return errors.New("could not get csrf_token")
		}

		images = extractImages(doc)
		if len(images) < 100 {
			l.Critical("画像の枚数が少なすぎます", nil)
			return errors.New("could not get csrf_token")
		}

		score.Increment(IndexGetScore)

		return nil
	})
	if err != nil {
		return
	}

	loadImages(s, images)
}

// トップページを開いて適当な部屋を開く（Ajaxじゃないのは「別タブで」開いたということにでもしておく）
func LoadRoomPage(s *session.Session) {
	var images []string
	var rooms []string

	err := s.Get("/", func(body io.Reader, l *fails.Logger) error {
		doc, err := goquery.NewDocumentFromReader(body)
		if err != nil {
			l.Add("ページのHTMLがパースできませんでした", err)
			return err
		}

		images = extractImages(doc)

		doc.Find("a").Each(func(_ int, selection *goquery.Selection) {
			if url, ok := selection.Attr("href"); ok {
				if strings.HasPrefix(url, "/rooms/") {
					rooms = append(rooms, url)
				}
			}
		})

		score.Increment(IndexGetScore)

		return nil
	})
	if err != nil {
		return
	}

	err = loadImages(s, images)
	if err != nil {
		return
	}

	roomURL := rooms[rand.Intn(len(rooms))]

	_ = s.Get(roomURL, func(body io.Reader, l *fails.Logger) error {

		// TODO: polylineのidを上で開いたSVGと比較するか？

		score.Increment(RoomGetScore)
		return nil
	})
}

// ページ内のCSRFトークンが毎回変わっていることをチェック
func CheckCSRFTokenRefreshed(s *session.Session) {
	var token string

	err := s.Get("/", func(body io.Reader, l *fails.Logger) error {
		doc, err := goquery.NewDocumentFromReader(body)
		if err != nil {
			l.Add("ページのHTMLがパースできませんでした", err)
			return err
		}

		token = extractCsrfToken(doc)

		if token == "" {
			l.Add("csrf_tokenが取得できませんでした", nil)
			return errors.New("could not get csrf_token")
		}

		score.Increment(IndexGetScore)

		return nil
	})
	if err != nil {
		return
	}

	_ = s.Get("/", func(body io.Reader, l *fails.Logger) error {
		doc, err := goquery.NewDocumentFromReader(body)
		if err != nil {
			l.Add("ページのHTMLがパースできませんでした", err)
			return err
		}

		newToken := extractCsrfToken(doc)

		if newToken == token {
			l.Critical("csrf_tokenが使いまわされています", nil)
			return errors.New("bad csrf_token")
		}

		score.Increment(IndexGetScore)

		return nil
	})
}
