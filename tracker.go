package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v7"
	"log"
	"net/http"
	"time"
)

type StreamTracker struct {
	password string
	mapper   *StreamMapper
	redis    *redis.Client
	mux      *http.ServeMux
}

func NewStreamTracker(mapper *StreamMapper, redis *redis.Client, password string) *StreamTracker {
	st := &StreamTracker{
		password: password,
		mapper:   mapper,
		redis:    redis,
		mux:      http.NewServeMux(),
	}
	st.mux.HandleFunc("/notify/publish", st.handlePublish)
	st.mux.HandleFunc("/notify/publish_done", st.handlePublishDone)
	st.mux.HandleFunc("/api/streams", st.handleStreams)
	st.mux.HandleFunc("/api/outputs", st.handleOutputs)
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
	for k, v := range st.mapper.GetStreams() {
		ls.Streams[k] = stream{Key: v, Live: active[v] == "1"}
	}
	if err := json.NewEncoder(w).Encode(ls); err != nil {
		log.Printf("Couldn't encode active streams: %v??\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (st *StreamTracker) handleOutputs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	if st.password != "" && r.URL.Query().Get("password") != st.password {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	out := struct {
		Outputs map[string]string `json:"outputs"`
	}{st.mapper.GetOutputs()}
	if err := json.NewEncoder(w).Encode(out); err != nil {
		log.Printf("Couldn't encode outputs: %v??\n", err)
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

	_, _ = w.Write([]byte(": hello\n\n"))
	w.(http.Flusher).Flush()

	const pingTime = 45 * time.Second
	pingChannel := time.After(pingTime)
	for {
		output := ""
		select {
		case message := <-pubsub.Channel():
			output = fmt.Sprintf("data: %s\n\n", message.Payload)
		case <-pingChannel:
			pingChannel = time.After(pingTime)
			output = ": ping\n\n"
		}

		_, err := w.Write([]byte(output))
		if err == nil {
			w.(http.Flusher).Flush()
		} else {
			log.Printf("write failed, dropping connection: %v", err)
			break
		}
	}
}
