package storage

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	bolt "go.etcd.io/bbolt"
	"log"
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
type instaPost struct {
	ImageBuf    []byte  `json:"data"`
	PublishDate time.Time `json:"publishDate"`
	Caption     string    `json:"caption"`
	ID          string    `json:"id"`
	User        string    `json:"user"`
	Location    string    `json:"location"`
}

type InstaPost struct {
	instaPost
	ImageBuf    []byte
}

func NewInstaPost(user, caption, location string, publishDate time.Time, buf []byte)  InstaPost {
	i := InstaPost{}
	i.User = user
	i.Caption = caption
	i.Location = location
	i.PublishDate = publishDate
	i.ImageBuf = buf

	h := md5.New()
	h.Write(buf)
	h.Write([]byte(user))
	i.ID = hex.EncodeToString(h.Sum(nil))
	return i
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

func (i *instaPost) Serialize() ([]byte, error) {
	return  json.Marshal(i)
}

func (i *instaPost) PublicInsta() InstaPost {
	return NewInstaPost(i.User, i.Caption, i.Location, i.PublishDate, i.ImageBuf )
}

func newInternalInstaPost(post InstaPost) instaPost {
	return instaPost{ID: post.ID, User:post.User, Caption: post.Caption, Location: post.Location, PublishDate: post.PublishDate, ImageBuf: post.ImageBuf }
}

type InstaSchedule struct {
	db             *bolt.DB
	boltBucketName []byte
	bolstBucket    *bolt.Bucket
	lock     sync.RWMutex
}

func NewInstaSchedule(path string) *InstaSchedule {
	s := InstaSchedule{}

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil
	}
	s.db = db
	s.boltBucketName = []byte("insta")
	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(s.boltBucketName)
		if bucket != nil {
			s.bolstBucket = bucket
			return nil
		}

		bucket, err = tx.CreateBucket(s.boltBucketName)
		s.bolstBucket = bucket
		return err
	})

	return &s
}

func (i *InstaSchedule) Set(post InstaPost) error {
	internalPost :=  newInternalInstaPost(post)
	buf, err := internalPost.Serialize()
	if err != nil {
		log.Println("Unable to serialize post", err)
		return err
	}

	log.Println("Storing post with ID", post.ID)
	i.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(i.boltBucketName)
		return bucket.Put([]byte(post.ID), buf)
	})

	return nil
}

func (i *InstaSchedule) Remove(id string) {
	i.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(i.boltBucketName)
		return bucket.Delete([]byte(id))
	})
}

func (i *InstaSchedule) Get(id string) (InstaPost, error) {
	p := instaPost{}
	keyBuf := []byte(id)
	var buf []byte
	err := i.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(i.boltBucketName)
		buf = bucket.Get(keyBuf)
		return nil
	})
	if err != nil {
		return InstaPost{}, err
	}
	err = json.Unmarshal(buf, &p)
	return p.PublicInsta(), err

}

func (i *InstaSchedule) GetAll() map[string]InstaPost {
	res := make(map[string]InstaPost)

	i.db.View(func(tx *bolt.Tx) error {
		// Assume bucket exists and has keys
		b := tx.Bucket(i.boltBucketName)

		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			p := instaPost{}
			err := json.Unmarshal(v, &p)
			if err != nil {
				log.Println("Error Unmarshal", err)
			}
			res[p.ID] = p.PublicInsta()
		}

		return nil
	})

	return res
}

func (i *InstaSchedule) Has(id string) bool {
	keyBuf := []byte(id)
	var buf []byte
	i.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(i.boltBucketName)
		buf = bucket.Get(keyBuf)
		return nil
	})

	if buf == nil {
		return false
	}
	return true
}

