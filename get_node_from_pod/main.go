package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/redis/go-redis/v9"
)

func main() {
	// get env variable named "HOSTNAME"

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}

	fmt.Println(hostname)
	fmt.Printf("MY_NODE_NAME: %s\n", os.Getenv("MY_NODE_NAME"))
	fmt.Printf("MY_POD_NAME: %s\n", os.Getenv("MY_POD_NAME"))
	fmt.Printf("MY_POD_NAMESPACE: %s\n", os.Getenv("MY_POD_NAMESPACE"))
	fmt.Printf("MY_POD_IP: %s\n", os.Getenv("MY_POD_IP"))
	fmt.Printf("MY_POD_SERVICE_ACCOUNT: %s\n", os.Getenv("MY_POD_SERVICE_ACCOUNT"))

	// establish a connection to a redis server at 10.101.102.102:6379
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "10.101.102.102:6379",
		Password: "", // no password set
		DB:       0,  // use default DB\
	})

	log.Printf("Redis client created for %s\n", "10.101.102.102:6379")

	// get outstanding_requests from redis
	val2, err := redisClient.Get(context.Background(), "outstanding_requests").Result()
	if err == redis.Nil {
		log.Println("outstanding_requests does not exist")
	} else if err != nil {
		log.Printf("Error: couldn't get variable from Redis\n")
	} else {
		log.Printf("outstanding_requests = %s\n", val2)
	}

	err = redisClient.Incr(context.Background(), "outstanding_requests").Err()
	if err != nil {
		log.Printf("Error: couldn't increment variable in Redis\n")
	}

	// get outstanding_requests from redis
	val2, err = redisClient.Get(context.Background(), "outstanding_requests").Result()
	if err == redis.Nil {
		log.Println("outstanding_requests does not exist")
	} else if err != nil {
		log.Printf("Error: couldn't get variable from Redis\n")
	} else {
		log.Printf("outstanding_requests = %s\n", val2)
	}

}
