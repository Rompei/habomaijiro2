package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/maxhawkins/google-places-api/places"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Tweet is object of tweet.
type Tweet struct {
	ID        string    `json:"id"`
	Idx       int       `json:"idx"`
	Text      string    `json:"text"`
	ImageURLs []string  `json:"imageUrls"`
	Date      time.Time `json:"date"`
	Place     *Place    `json:"place"`
	Menu      string    `json:"menu"`
	Price     int       `json:"price"`
	Feel      string    `json:"feel"`
}

// Place is object for place of Jiro.
type Place struct {
	Name    string  `json:"name"`
	Address string  `json:"address"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
}

// NewPlace is constructor of Place.
func NewPlace(name string) *Place {
	return &Place{
		Name: name,
	}
}

func main() {
	var p = flag.Bool("p", false, "Add place info in detail")
	var f = flag.String("f", "", "Existing file")
	flag.Parse()
	var tweets []Tweet
	var err error
	if *f == "" {
		tweets, err = getTweetFromFile("twitter.html")
	} else {
		tweets, err = getTweetFromArchive(*f)
	}

	if err != nil {
		panic(err)
	}

	tweets = refacter(tweets)
	if *p {
		getPlace(tweets)
	}

	dump(tweets, "data/habomai.json")
}

func getTweetFromArchive(fname string) (tweets []Tweet, err error) {
	b, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(b, &tweets)
	if err != nil {
		return nil, err
	}

	return tweets, nil
}

func getTweetFromFile(fname string) (tweets []Tweet, err error) {
	f, err := os.Open(fname)
	if err != nil {
		return
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return
	}

	idx := 0
	doc.Find(".js-stream-tweet").Each(func(i int, s *goquery.Selection) {
		if s.HasClass("separated-module") || s.HasClass("has-profile-promoted-tweet") {
			return
		}
		var t Tweet
		t.Idx = idx
		idx++
		analizeText(&t, s.Find(".tweet-text").Text())

		// 日時取得
		if ts, isExist := s.Find(".js-short-timestamp").Attr("data-time"); isExist {
			if fts, err := strconv.ParseInt(ts, 10, 64); err == nil {
				t.Date = time.Unix(fts, 0)
			}
		}

		// 画像取得
		s.Find(".js-adaptive-photo").Each(func(i int, s *goquery.Selection) {
			if url, isExist := s.Attr("data-image-url"); isExist {
				t.ImageURLs = append(t.ImageURLs, url)
			} else {
				fmt.Println("image")
			}
		})

		// ID取得
		t.ID, _ = s.Attr("data-tweet-id")
		if t.ID != "" {
			tweets = append(tweets, t)
		}
	})

	return
}

func analizeText(t *Tweet, text string) (err error) {
	// 本文取得
	t.Text = text

	splitedText := strings.Split(text, "、")

	// 店名取得
	if len(splitedText) >= 2 {
		t.Place = NewPlace(splitedText[1])
	}

	// 価格取得
	if len(splitedText) >= 3 {
		re1, err := regexp.Compile(`\d*YEN`)
		re2, err := regexp.Compile(`YEN`)
		if err != nil {
			return err
		}
		t.Price, err = strconv.Atoi(re2.ReplaceAllString(re1.FindString(splitedText[2]), ""))
		if err != nil {
			return err
		}

		// メニュー取得
		re3, err := regexp.Compile(`.*YEN`)
		if err != nil {
			return err
		}
		t.Menu = re1.ReplaceAllString(re3.FindString(splitedText[2]), "")
	}

	// 感想取得
	re4, err := regexp.Compile(`.*YEN|https?://[\w/:%#\$&\?\(\)~\.=\+\-]+|pic\.twitter\.com.*`)
	t.Feel = strings.TrimSpace(re4.ReplaceAllString(text, ""))

	return
}

func sendSearch(service *places.Service, tweet *Tweet, wg *sync.WaitGroup) (err error) {
	defer wg.Done()
	if tweet.Place == nil {
		return
	}
	call := service.TextSearch(tweet.Place.Name)
	call.Language = "ja"
	resp, err := call.Do()
	if err != nil {
		fmt.Println(err)
		return
	}
	if len(resp.Results) > 0 {
		tweet.Place.Address = resp.Results[0].FormattedAddress
		tweet.Place.Lat = resp.Results[0].Geometry.Location.Lat
		tweet.Place.Lng = resp.Results[0].Geometry.Location.Lng
	}
	return
}

func getPlace(tweets []Tweet) (err error) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	var wg sync.WaitGroup
	service := places.NewService(http.DefaultClient, os.Getenv("PLACE_API_KEY"))
	for i := range tweets {
		if tweets[i].Place.Address == "" {
			wg.Add(1)
			go sendSearch(service, &tweets[i], &wg)
		}
	}
	wg.Wait()
	return
}

func refacter(tweets []Tweet) (res []Tweet) {
	for _, t := range tweets {
		if t.Place == nil || t.Price == 0 || t.Menu == "" {
			continue
		}
		res = append(res, t)
	}
	return
}

func p(tweet *Tweet) {
	fmt.Println("Text", tweet.Text)
	fmt.Println("Feel", tweet.Feel)
	fmt.Println("Price", tweet.Price)
	fmt.Println("Menu", tweet.Menu)
	fmt.Println("Date", tweet.Date)
	fmt.Println()
}

func dump(tweets []Tweet, fname string) (err error) {
	b, err := json.MarshalIndent(tweets, "", "\t")
	if err != nil {
		return
	}
	if err = ioutil.WriteFile(fname, b, os.ModePerm); err != nil {
		if err = os.MkdirAll(filepath.Dir(fname), os.ModePerm); err != nil {
			return err
		}
		if err = ioutil.WriteFile(fname, b, os.ModePerm); err != nil {
			return err
		}
	}
	return
}
