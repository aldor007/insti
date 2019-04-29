package main

import (
	"bytes"
	"crypto/md5"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/ahmdrz/goinsta"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
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

type InstaPost struct {
	imageBuf    []byte
	PublishDate time.Time `json:"publishDate"`
	Caption     string    `json:"caption"`
	ID          string    `json:"id"`
	User        string    `json:"user"`
}

func (i *InstaPost) MarshalJSON() ([]byte, error) {
	type Alias InstaPost
	return json.Marshal(&struct {
		*Alias
		PublisDate string `json:"publishDate"`
	}{
		Alias:      (*Alias)(i),
		PublisDate: i.PublishDate.Format(time.RFC1123),
	})
}

type InstaSchedule struct {
	schedule map[string]InstaPost
	lock     sync.RWMutex
}

func NewInstaSchedule() *InstaSchedule {
	s := InstaSchedule{}
	s.schedule = make(map[string]InstaPost)
	return &s
}

func (i *InstaSchedule) Add(post InstaPost) {
	lock.Lock()
	defer lock.Unlock()
	h := md5.New()
	h.Write(post.imageBuf)
	post.ID = hex.EncodeToString(h.Sum(nil))
	i.schedule[post.ID] = post
}

func (i *InstaSchedule) Remove(id string) {
	lock.Lock()
	defer lock.Unlock()
	delete(i.schedule, id)
}

func (i *InstaSchedule) Get(id string) InstaPost {
	lock.RLock()
	defer lock.RUnlock()
	return i.schedule[id]
}

func (i *InstaSchedule) GetAll() map[string]InstaPost {
	lock.RLock()
	defer lock.RUnlock()
	return i.schedule
}

func (i *InstaSchedule) Has(id string) bool {
	lock.RLock()
	defer lock.RUnlock()
	_, ok := i.schedule[id]
	return ok
}

var lock sync.RWMutex

var insta *goinsta.Instagram
var users map[string]*goinsta.Instagram
var postSchedule *InstaSchedule

func handleNewUser(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()

	if err != nil {
		log.Println("Error parsing form", err)
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	login := r.FormValue("login")
	password := r.FormValue("password")

	if login == "" || password == "" {
		log.Println("invalid data", login, password)
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return

	}

	localInsta := goinsta.New(login, password)
	if err := localInsta.Login(); err != nil {
		log.Println("Error login to instagram", err)
		http.Error(w, "Error login to instagram", http.StatusBadRequest)
		return
	}

	users[login] = localInsta

	fmt.Fprintf(w, "user "+login+" added to local db")
}

func handleUser(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		handleNewUser(w, r)
		return
	} else {
		keys := make([]string, 0, len(users))
		for k := range users {
			keys = append(keys, k)
		}

		w.Header().Set("content-type", "application/json")
		d, _ := json.Marshal(keys)
		w.Write(d)
	}
}

func handlePostData(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(32 << 20)

	if err != nil {
		log.Println("Error parsing form", err)
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	publishDate, err := strconv.ParseInt(r.PostFormValue("publishDate"), 10, 64)
	if err != nil {
		log.Println("Error parsing  publishDate", err)
		http.Error(w, "Error parsing publishDate", http.StatusBadRequest)
		return
	}

	user := r.PostFormValue("user")

	publishDate = publishDate / 1000
	tm := time.Unix(publishDate, 0)
	log.Println("Run at ", tm, " after", tm.Sub(time.Now()))
	caption := r.PostFormValue("caption")
	file, _, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "image upload error", http.StatusInternalServerError)
		return
	}
	imageBuf, err := ioutil.ReadAll(file)
	if user != "" {
		var ok bool
		_, ok = users[user]
		if !ok {
			log.Println("Error unknown user", user)
			http.Error(w, "Error unknown user", http.StatusBadRequest)
			return
		}
	}
	post := InstaPost{imageBuf: imageBuf, Caption: caption, User: user, PublishDate: tm}
	postSchedule.Add(post)
	file.Close()

}

func handleGetSchedule(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("content-type", "application/json")
	data := postSchedule.GetAll()
	list := make([]InstaPost, 0)
	for _, v := range data {
		list = append(list, v)
	}

	jsonData := make(map[string][]InstaPost)
	jsonData["data"] = list
	d, _ := json.Marshal(jsonData)
	w.Write(d)

}

func handleGetImage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	post := postSchedule.Get(vars["id"])
	if post.imageBuf == nil {
		http.Error(w, "No image", http.StatusBadRequest)
		return
	}

	w.Header().Set("content-type", "image/jpeg")
	w.Write(post.imageBuf)
}

func handleRemovePost(_ http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	postSchedule.Remove(vars["id"])
}

func publishImage(post InstaPost) {
	errorCounter := 0
	var userInsta *goinsta.Instagram
	user := post.User
	if user == "" {
		userInsta = insta
	} else {
		var ok bool
		userInsta, ok = users[user]
		if !ok {
			log.Println("Error unknown user", user)
			return
		}
	}

	for i := 0; i < 3; i++ {
		if !postSchedule.Has(post.ID) {
			log.Println("Skip publish", post.ID)
			return
		}

		_, err := userInsta.UploadPhoto(bytes.NewReader(post.imageBuf), post.Caption, 100, 1)
		if err != nil && errorCounter < 3 {
			errorCounter++
			log.Println("image upload error", err)
		} else {
			log.Println("Published image")
			postSchedule.Remove(post.ID)
			return

		}
	}
}
func postWorker(postsIn *InstaSchedule) {

	ticker := time.NewTicker(time.Minute * 1)

	go func() {
		for {

			select {
			case <-ticker.C:
				posts := postsIn.GetAll()
				log.Println("postWorker schedule len", len(posts))
				for _, value := range posts {
					if time.Now().Sub(value.PublishDate).Seconds() >= 0 {
						publishImage(value)
					}
				}

			}

		}
	}()
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

	users = make(map[string]*goinsta.Instagram)
	postSchedule = NewInstaSchedule()

	if userName == nil || *userName == "" {
		panic("Missing required parameter")
	}

	if os.Getenv("INSTA_USERNAME") == "" || os.Getenv("INSTA_PASSWORD") == "" {
		panic("Missing env variables")
	}

	log.Println("Collecting data for ", *userName)
	log.Println("Server listen", *addr)
	prometheus.MustRegister(followersCount, likesCount, commentsCount, errorsMonitoring)
	var err error
	insta, err = goinsta.Import("~/.goinsta2")
	if err != nil {
		insta = goinsta.New(os.Getenv("INSTA_USERNAME"), os.Getenv("INSTA_PASSWORD"))
	}

	if err := insta.Login(); err != nil {
		log.Println("login error", err)
		return
	}
	errorCounter := 0

	file, err := os.OpenFile(*filePath, os.O_APPEND|os.O_WRONLY, 0600)

	if err != nil {
		file, err = os.Create(*filePath)
		if err != nil {
			panic(err)
		}
	}
	csvFile := csv.NewWriter(file)

	postWorker(postSchedule)
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
				log.Println("Error writing to csv", err)
				file, err = os.OpenFile(*filePath, os.O_APPEND|os.O_WRONLY, 0600)

				if err != nil {
					file, err = os.Create(*filePath)
					if err != nil {
						panic(err)
					}
				}
				csvFile = csv.NewWriter(file)
			}
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

	}, 5+errorCounter)

	insta.Export("~/.goinsta")

	flag.Parse()
	fs := http.FileServer(http.Dir("static"))
	rtr := mux.NewRouter()
	rtr.Handle("/metrics", promhttp.Handler())
	rtr.HandleFunc("/post", handlePostData).Methods("POST")
	rtr.HandleFunc("/post/{id}", handleRemovePost).Methods("DELETE")
	rtr.HandleFunc("/schedule", handleGetSchedule).Methods("GET")
	rtr.HandleFunc("/image/{id}", handleGetImage).Methods("GET")
	rtr.HandleFunc("/user", handleUser)
	rtr.PathPrefix("/").Handler(fs)
	http.Handle("/", rtr)

	log.Fatal(http.ListenAndServe(*addr, nil))
}
