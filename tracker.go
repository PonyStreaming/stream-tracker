package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v7"
	"log"
	"net/http"
)

type StreamTracker struct {
	password string
	mapper   *StreamMapper
	redis    *redis.Client
	mux      *http.ServeMux
}

func NewStreamTracker(mapper *StreamMapper, redis *redis.Client, password string) *StreamTracker {
	st := new(StreamTracker)
	st.password = password
	st.mapper = mapper
	st.redis = redis
	st.mux = http.NewServeMux()
	st.mux.HandleFunc("/notify/publish", st.handlePublish)
	st.mux.HandleFunc("/notify/publish_done", st.handlePublishDone)
	st.mux.HandleFunc("/api/streams", st.handleStreams)
	st.mux.HandleFunc("/api/stream_updates", st.handleStreamUpdates)
	return st
}

func (st *StreamTracker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	st.mux.ServeHTTP(w, r)
}

func (st *StreamTracker) handlePublish(w http.ResponseWriter, r *http.Request) {
	key := r.PostFormValue("name")
	if !st.mapper.HasStreamKey(key) {
		http.Error(w, "Invalid key", http.StatusNotFound)
		return
	}
	if err := st.redis.HSet("active_streams", key, "1").Err(); err != nil {
		log.Printf("Failed to update stream activity: %v\n", err)
	}
	name := st.mapper.StreamNameFromKey(key)
	if name != "" {
		if err := st.redis.Publish("stream_activity", "1|"+name).Err(); err != nil {
			log.Printf("Failed to publish stream update: %v\n", err)
		}
	}
	_, _ = w.Write([]byte("ok"))
}

func (st *StreamTracker) handlePublishDone(w http.ResponseWriter, r *http.Request) {
	key := r.PostFormValue("name")
	if err := st.redis.HDel("active_streams", key).Err(); err != nil {
		log.Printf("Failed to update stream activity: %v\n", err)
	}
	name := st.mapper.StreamNameFromKey(key)
	if name != "" {
		if err := st.redis.Publish("stream_activity", "0|"+name).Err(); err != nil {
			log.Printf("Failed to publish stream update: %v\n", err)
		}
	}
	_, _ = w.Write([]byte("ok"))
}

type stream struct {
	Key  string `json:"key"`
	Live bool   `json:"live"`
}

type liveStreams struct {
	Streams map[string]stream `json:"streams"`
}

func (st *StreamTracker) handleStreams(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	if st.password != "" && r.URL.Query().Get("password") != st.password {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	active, err := st.redis.HGetAll("active_streams").Result()
	if err != nil {
		log.Printf("Couldn't look up active streams: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ls := liveStreams{Streams: map[string]stream{}}
	for k, v := range st.mapper.mapping {
		ls.Streams[k] = stream{Key: v, Live: active[v] == "1"}
	}
	if err := json.NewEncoder(w).Encode(ls); err != nil {
		log.Printf("Couldn't encode active streams: %v??\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (st *StreamTracker) handleStreamUpdates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if st.password != "" && r.URL.Query().Get("password") != st.password {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	pubsub := st.redis.Subscribe("stream_activity")
	defer pubsub.Close()

	for {
		message := <-pubsub.Channel()
		_, err := w.Write([]byte(fmt.Sprintf("data: %s\n\n", message.Payload)))
		if err == nil {
			w.(http.Flusher).Flush()
		}
		if err != nil {
			log.Printf("write failed, dropping connection: %v", err)
			break
		}
	}
}
