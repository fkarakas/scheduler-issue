package main

import (
	"context"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	_ "github.com/mattn/go-sqlite3"
)

var (
	multibrokers = "localhost:9092,localhost:9093"
	brokers      = multibrokers
)

func CreateTopic(name string) error {
	adm, err := kafka.NewAdminClient(&kafka.ConfigMap{
		"bootstrap.servers": brokers,
	})
	if err != nil {
		return fmt.Errorf("failed to create admin client: %v", err)
	}
	defer adm.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	specs := []kafka.TopicSpecification{
		{
			Topic:             name,
			NumPartitions:     2,
			ReplicationFactor: 1,
			// Config: map[string]string{
			// 	"cleanup.policy": "compact",
			// 	"retention.ms":   "-1",
			// },
		},
	}

	topics := []string{name}
	results, err := adm.DeleteTopics(ctx, topics)
	if err != nil {
		return fmt.Errorf("failed to delete topics %v: %v", topics, err)
	}
	for _, result := range results {
		fmt.Printf("delete %v\n", result.String())
	}
	time.Sleep(5 * time.Second)

	results, err = adm.CreateTopics(ctx, specs, kafka.SetAdminOperationTimeout(10*time.Second))
	if err != nil {
		return fmt.Errorf("failed to create topics %v: %v", specs, err)
	}

	for _, result := range results {
		fmt.Printf("create %v\n", result.String())
	}

	return nil
}

func main() {
	CreateTopic("schedules")
	CreateTopic("timers")
	CreateTopic("history")
}
