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
	"encoding/csv"
	"strconv"
	"regexp"
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
	followersCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "instagram_followers_count",
		Help: "followers count for give account",
	},
		[]string{"account"},
	)

	errorsMonitoring = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "instagram_errors_count",
		Help: "instrgram API errors count",
	})

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
	tagRegexp = regexp.MustCompile("#[a-z_]+")

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
	filePath := flag.String("csvPath", "", "CSV file path")
	flag.Parse()

	if userName == nil || *userName == "" {
		panic("Missing required parameter")
	}

	if os.Getenv("INSTA_USERNAME") == "" || os.Getenv("INSTA_PASSWORD") == "" {
		panic("Missing env variables")
	}

	log.Println("Collecting data for ", *userName)
	log.Println("Server listen", *addr)
	prometheus.MustRegister(followersCount, likesCount, commentsCount, errorsMonitoring)

	insta, err := goinsta.Import("~/.goinsta2")
	if err != nil {
		insta = goinsta.New(os.Getenv("INSTA_USERNAME"), os.Getenv("INSTA_PASSWORD"))
	}

	if err := insta.Login(); err != nil {
		fmt.Println("login error", err)
		return
	}
	//lock := sync.RWMutex{}
	//collectedData := make(map[string]MediaData)
	errorCounter := 0

	file, err := os.OpenFile(*filePath, os.O_APPEND|os.O_WRONLY, 0600)

	if err != nil {
		file, err = os.Create(*filePath)
		if err != nil  {
			panic(err)
		}
	}
	csvFile := csv.NewWriter(file)

	setInterval(func() {
		user, err := insta.Profiles.ByName(*userName)

		if err != nil {
			log.Println("Error getting user", err)
			errorsMonitoring.Inc()
			errorCounter++
			return
		}

		followersCount.WithLabelValues(*userName).Set(float64(user.FollowerCount))
		media := user.Feed()
		media.Next()
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		for _, item := range media.Items {
			likesCount.WithLabelValues(item.Code).Set(float64(item.Likes))
			commentsCount.WithLabelValues(item.Code).Set(float64(item.CommentCount))
			err = csvFile.Write([]string{timestamp, item.Code, strconv.Itoa(item.Likes), strconv.Itoa(item.CommentCount), strconv.Itoa(user.FollowerCount),
			strconv.Itoa(len(item.Caption.Text)), strconv.Itoa(len(tagRegexp.FindAllStringIndex(item.Caption.Text, -1))), strconv.Itoa(int(item.TakenAt))})
			if err != nil {
				log.Println("Error wwriting to csv", err)
				file, err = os.OpenFile(*filePath, os.O_APPEND|os.O_WRONLY, 0600)

				if err != nil {
					file, err = os.Create(*filePath)
					if err != nil  {
						panic(err)
					}
				}
				csvFile = csv.NewWriter(file)
			}
			//lock.Lock()
			//collectedData[item.Code] = MediaData{item.Likes, item.CommentCount}
			//lock.Unlock()
			fmt.Println(item.Code, item.Likes, item.CommentCount)
		}
		err = user.Sync()
		if err != nil {
			log.Println("Sync error", err)
			errorCounter++
		}

		if errorCounter > 4 {
			errorCounter = 0
		}

		csvFile.Flush()
		if err != nil {
			log.Println("Error", err)
		}

	}, 5 + errorCounter)

	insta.Export("~/.goinsta")

	flag.Parse()
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*addr, nil))
}
