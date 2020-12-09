package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/TheForgotten69/goinsta/v2"
	"github.com/aldor007/insti/storage"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/jasonlvhit/gocron"

	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"
)

var lock sync.RWMutex

var insta *goinsta.Instagram
var users map[string]*goinsta.Instagram
var postSchedule *storage.InstaSchedule
var followersStore map[string]struct{}
var unfollowers []string
var prevFollowCount int

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
	post := storage.NewInstaPost(user, caption, r.PostFormValue("location"), tm, imageBuf)
	err = postSchedule.Set(post)
	if err != nil {
		log.Println("Unable to store image", err)
		http.Error(w, "image save error", http.StatusInternalServerError)
		return

	}
	file.Close()

}

func handleGetSchedule(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("content-type", "application/json")
	data := postSchedule.GetAll()
	list := make([]storage.InstaPost, 0)
	for _, v := range data {
		list = append(list, v)
	}

	jsonData := make(map[string][]storage.InstaPost)
	jsonData["data"] = list
	d, _ := json.Marshal(jsonData)
	w.Write(d)

}

func handleGetImage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	post, err := postSchedule.Get(vars["id"])
	if post.ImageBuf == nil || err != nil {
		log.Println("Error getting image", err)
		http.Error(w, "No image", http.StatusBadRequest)
		return
	}

	w.Header().Set("content-type", "image/jpeg")
	w.Header().Set("cache-control", "max-age=3600, private")
	w.Write(post.ImageBuf)
}

func handleFollowers(w http.ResponseWriter, r *http.Request) {
	unfollowersStr, _ := json.Marshal(unfollowers)
	followersArr := make([]string, 0)
	for f, _ := range followersStore {
		followersArr = append(followersArr, f)
	}
	followerStr, _ := json.Marshal(followersArr)
	res := fmt.Sprintf("Followers count %d followers %s recent unfollows %s", prevFollowCount, string(followerStr), string(unfollowersStr))
	fmt.Fprint(w, res)
}

func handleRemovePost(_ http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	postSchedule.Remove(vars["id"])
}

func publishImage(post storage.InstaPost) {
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

		item, err := userInsta.UploadPhoto(bytes.NewReader(post.ImageBuf), post.Caption, 100, 1)
		if err != nil && errorCounter < 3 {
			errorCounter++
			log.Println("image upload error", err)
			time.Sleep(time.Minute * 5)
		} else {
			item.Location.City = post.Location
			log.Println("Published image")
			postSchedule.Remove(post.ID)
			return

		}
	}
}
func postWorker(postsIn *storage.InstaSchedule) {

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
func collectStats(userName *string,  ) {
	user, err := insta.Profiles.ByName(*userName)

	if err != nil {
		log.Println("Error getting user", err)
		errorsMonitoring.Inc()
		return
	}

	followersCount.WithLabelValues(*userName).Set(float64(user.FollowerCount))
	media := user.Feed()
	media.Next()
	for _, item := range media.Items {
		likesCount.WithLabelValues(item.Code).Set(float64(item.Likes))
		commentsCount.WithLabelValues(item.Code).Set(float64(item.CommentCount))
	}
	err = user.Sync()
	if err != nil {
		log.Println("Sync error", err)
	}

	if err != nil {
		log.Println("Error", err)
	}
}

func collectFollowers(userName *string) {
	user, err := insta.Profiles.ByName(*userName)
	if err != nil {
		log.Println("Error getting user", err)
		errorsMonitoring.Inc()
		return
	}
	err = user.Sync()
	if err != nil {
		log.Println("Sync error", err)
	}
	currentFollowers := make([]string, 0)

	followersCount.WithLabelValues(*userName).Set(float64(user.FollowerCount))
	followers := user.Followers()
	if user.FollowerCount == prevFollowCount {
		return
	}

	log.Println("current followers", user.FollowerCount, prevFollowCount)
	prevFollowCount = user.FollowerCount
	log.Println(followers.PageSize, len(followers.Users))
	followers.Next()
	for _, u := range followers.Users {
		currentFollowers = append(currentFollowers, u.Username)
	}
	for followers.Next() {
		for _, u := range followers.Users {
			currentFollowers = append(currentFollowers, u.Username)
			log.Println("Adding2", u.Username)
		}
	}

	log.Println("Checking current followers checker", len(currentFollowers))
	for _, u := range currentFollowers {
		if _, ok := followersStore[u]; !ok {
			followersStore[u] = struct{}{}
			log.Println("User added to follower store", u)
		}
	}


	for  uInStore, _ := range followersStore {
		found := false
		for _, u := range currentFollowers {
			if uInStore == u {
				found = true
				log.Println("User found in store", uInStore)
				break
			}
		}

		if !found {
			log.Println("User unfollow", uInStore)
			unfollowers = append(unfollowers, uInStore)
			delete(followersStore, uInStore)
		}
	}

}

func main() {
	addr := flag.String("listen", ":8080", "The address to listen on for HTTP requests.")
	userName := flag.String("user", "", "User name to observe")
	dbPath := flag.String("dbPath", "./data", "CSV file path")
	flag.Parse()

	users = make(map[string]*goinsta.Instagram)
	followersStore = make(map[string]struct{})
	unfollowers = make([]string, 0)
	postSchedule = storage.NewInstaSchedule(*dbPath)

	if userName == nil || *userName == "" {
		panic("Missing required parameter username")
	}

	if os.Getenv("INSTA_USERNAME") == "" || os.Getenv("INSTA_PASSWORD") == "" {
		panic("Missing env variables with insta user/password for collect user")
	}
	permStore := os.Getenv("INSTA_DATA_PATH")
	log.Println("Collecting data for ", *userName)
	log.Println("Server listen", *addr)
	prometheus.MustRegister(followersCount, likesCount, commentsCount, errorsMonitoring)
	var err error
	insta, err = goinsta.Import(permStore + ".goinsta")
	if err != nil {
		insta = goinsta.New(os.Getenv("INSTA_USERNAME"), os.Getenv("INSTA_PASSWORD"))
		if err := insta.Login(); err != nil {
			log.Println("login error", err)
			return
		}
	}

	insta.Export(permStore + ".goinsta")

	collectFollowers(userName)
	//gocron.Every(1).Hours().Do(collectStats, userName)
	gocron.Every(7).Hours().Do(collectFollowers, userName)
	go gocron.Start()

	flag.Parse()
	fs := http.FileServer(http.Dir("static"))
	rtr := mux.NewRouter()
	rtr.Handle("/metrics", promhttp.Handler())
	rtr.HandleFunc("/post", handlePostData).Methods("POST")
	rtr.HandleFunc("/post/{id}", handleRemovePost).Methods("DELETE")
	rtr.HandleFunc("/schedule", handleGetSchedule).Methods("GET")
	rtr.HandleFunc("/image/{id}", handleGetImage).Methods("GET")
	rtr.HandleFunc("/user", handleUser)
	rtr.HandleFunc("/followers", handleFollowers)
	rtr.PathPrefix("/").Handler(fs)
	http.Handle("/", rtr)

	log.Fatal(http.ListenAndServe(*addr, nil))
}
