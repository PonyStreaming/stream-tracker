package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-redis/redis/v7"
)

type config struct {
	bind     string
	config   string
	password string
	redisURL string
}

func parseFlags() (config, error) {
	c := config{}
	flag.StringVar(&c.bind, "bind", "0.0.0.0:8080", "The host:port to bind to.")
	flag.StringVar(&c.config, "config", "", "The location of the config file.")
	flag.StringVar(&c.password, "read-password", "", "The password to read keys from this service.")
	flag.StringVar(&c.redisURL, "redis-url", "", "URL for connecting to redis")
	flag.Parse()

	if c.config == "" {
		return c, fmt.Errorf("you must specify --config")
	}
	if c.redisURL == "" {
		return c, fmt.Errorf("you must specify --redis-url")
	}
	return c, nil
}

func main() {
	c, err := parseFlags()
	if err != nil {
		log.Println(err)
		os.Exit(2)
	}

	redisOptions, err := redis.ParseURL(c.redisURL)
	if err != nil {
		log.Printf("Couldn't parse redis URL: %v\n", err)
		os.Exit(1)
	}

	r := redis.NewClient(redisOptions)
	if _, err := r.Ping().Result(); err != nil {
		log.Printf("Couldn't communicate with redis: %v.\n", err)
		os.Exit(1)
	}

	sm, err := NewStreamMapper(c.config)
	if err != nil {
		log.Printf("Couldn't parse stream mapping: %v.\n", err)
		os.Exit(1)
	}
	sm.Run()

	st := NewStreamTracker(sm, r, c.password)
	http.Handle("/", st)
	log.Fatal(http.ListenAndServe(c.bind, nil))
}
