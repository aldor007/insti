package main

import (
	"flag"
	"fmt"
	"github.com/ahmdrz/goinsta"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"os"
	"sync"
	"log"
	"time"
)

type MediaData struct {
	likes         int
	commentsCount int
}

type InstaData struct {
	data map[string]MediaData
	lock sync.RWMutex
}

var (
	fallowesCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "instagram_fallowers_count",
		Help: "fallowes count for give account",
	},
		[]string{"account"},
	)

	likesCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "instagram_likes_count",
		Help: "likes count for given image",
	},
		[]string{"imageId"},
	)

	commentsCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "instagram_comments_count",
		Help: "comments count for given image",
	},
		[]string{"imageId"},
	)
)

func setInterval(someFunc func(), minutes int) chan bool {

	interval := time.Duration(minutes) * time.Minute

	ticker := time.NewTicker(interval)
	clear := make(chan bool)

	someFunc()

	go func() {
		for {

			select {
			case <-ticker.C:
				someFunc()
			case <-clear:
				ticker.Stop()
				return
			}

		}
	}()

	return clear

}

func main() {
	addr := flag.String("listen", ":8080", "The address to listen on for HTTP requests.")
	userName := flag.String("user", "", "User name to observe")
	flag.Parse()

	if userName == nil || *userName == "" {
		panic("Missing required paramter")
	}

	if os.Getenv("INSTA_USERNAME") == "" || os.Getenv("INSTA_PASSWORD") == "" {
		panic("Missing env variables")
	}

	log.Println("Collecting data for ", *userName)
	log.Println("Server listen", *addr)
	prometheus.MustRegister(fallowesCount, likesCount, commentsCount)

	insta, err := goinsta.Import("~/.goinsta2")
	if err != nil {
		insta = goinsta.New(os.Getenv("INSTA_USERNAME"), os.Getenv("INSTA_PASSWORD"))
	}

	if err := insta.Login(); err != nil {
		fmt.Println("login error", err)
		return
	}
	maxItems := 5
	//lock := sync.RWMutex{}
	//collectedData := make(map[string]MediaData)

	setInterval(func() {
		user, err := insta.Profiles.ByName(*userName)
		if err != nil {
			log.Println("Error getting user", err)
		}

		fallowesCount.WithLabelValues(*userName).Set(float64(user.FollowerCount))

		media := user.Feed()
		media.Next()
		i := 0
		for _, item := range media.Items {
			likesCount.WithLabelValues(item.Code).Set(float64(item.Likes))
			commentsCount.WithLabelValues(item.Code).Set(float64(item.Likes))
			//lock.Lock()
			//collectedData[item.Code] = MediaData{item.Likes, item.CommentCount}
			//lock.Unlock()
			fmt.Println(item.Code)
			i++
			fmt.Println(i, maxItems)
			if i > maxItems {
				break
			}
		}
		err = user.Sync()
		if err != nil {
			log.Println("Sync error", err)
		}

	}, 5)

	insta.Export("~/.goinsta")

	flag.Parse()
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*addr, nil))
}
