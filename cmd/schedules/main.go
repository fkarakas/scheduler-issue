package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

var (
	itoa         = strconv.Itoa
	topic        = "schedules"
	max          = 10000
	monobroker   = "localhost:9092"
	multibrokers = "localhost:9092,localhost:9093"
	brokers      = multibrokers
)

func produceSchedules(size int) {
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": brokers,
	})
	if err != nil {
		panic(err)
	}

	defer p.Close()

	done := make(chan bool)

	// Delivery report handler for produced messages
	go func() {
		defer func() {
			done <- true
		}()

		count := 0
		for e := range p.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					fmt.Printf("Delivery failed: %v\n", ev.TopicPartition)
				} else {
					count++
					fmt.Printf("Delivered message to %v (count=%v)\n", ev.TopicPartition, count)
					if count == size {
						fmt.Printf("produceAll: size reached\n")
						return
					}
				}
			}
		}
	}()

	// Produce messages to topic (asynchronously)
	for i := 1; i <= size; i++ {
		key := uuid.NewMD5(uuid.NameSpaceOID, []byte(itoa(i))).String()
		msg := &kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Key:            []byte(fmt.Sprintf("%v-schedule", key)),
			Headers: []kafka.Header{
				{
					Key:   "scheduler-epoch",
					Value: []byte(fmt.Sprintf("%v", time.Now().Unix()+10)),
				},
				{
					Key:   "scheduler-target-topic",
					Value: []byte("timers"),
				},
				{
					Key:   "scheduler-target-key",
					Value: []byte(key),
				},
			},
			Value: []byte(fmt.Sprintf("message payload for key: %s", string(key))),
		}

		p.Produce(msg, nil)
	}

	// Wait for message deliveries before shutting down
	p.Flush(15 * 1000)

	<-done
}

func consumeMessages(topics []string, size int) chan *kafka.Message {
	out := make(chan *kafka.Message, 1000)

	go func() {
		c, err := kafka.NewConsumer(&kafka.ConfigMap{
			"bootstrap.servers": brokers,
			"group.id":          "cg-test",
			"auto.offset.reset": "earliest",
		})
		defer func() {
			c.Close()
			close(out)
		}()

		if err != nil {
			panic(err)
		}

		c.SubscribeTopics(topics, nil)
		count := 0
		for {
			msg, err := c.ReadMessage(-1)
			if err == nil {
				count++
				fmt.Printf("Message on %s: %s %v\n", msg.TopicPartition, string(msg.Value), count)
				out <- msg
				if count == size {
					fmt.Printf("consumeMessages: size reached\n")
					return
				}
			} else {
				// The client will automatically try to recover from all errors.
				fmt.Printf("Consumer error: %v (%v)\n", err, msg)
			}
		}
	}()

	return out
}

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

type Store struct {
	db *sql.DB
}

func NewStore(path string) (Store, error) {
	os.Remove(path)

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return Store{}, err
	}

	sqlStmt := `
		create table message(
			key text not null check (trim(key) != ""),
			topic text not null check (trim(topic) != ""),
			timestamp timestamp not null,
			headers text,
			partition integer not null,
			offset integer not null,
			value text
		);
		`
	_, err = db.Exec(sqlStmt)
	if err != nil {
		return Store{}, err
	}

	return Store{db: db}, nil
}

func (s Store) InsertMessage(msg *kafka.Message) error {
	fmt.Printf("insert %v\n", msg)
	var headers any
	if len(msg.Headers) != 0 {
		h, err := json.Marshal(msg.Headers)
		if err != nil {
			return err
		}
		headers = h
	}
	var value any
	if len(msg.Value) != 0 {
		value = string(msg.Value)
	}
	fmt.Printf("inserting %v\n", msg)
	_, err := s.db.Exec("insert into message(key, topic, timestamp, headers, partition, offset, value) values(?, ?, ?, ?, ?, ?, ?);",
		string(msg.Key), msg.TopicPartition.Topic, msg.Timestamp.Unix(), headers, msg.TopicPartition.Partition, msg.TopicPartition.Offset, value)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	CreateTopic("schedules")
	CreateTopic("timers")
	produceSchedules(max)

	s, err := NewStore("sqlite.db")
	if err != nil {
		panic(err)
	}
	index := 0
	for m := range consumeMessages([]string{"schedules", "timers"}, 3*max) {
		index++
		fmt.Printf("received message: key=%s partition=%v topic=%v\n", m.Key, m.TopicPartition.Partition, *m.TopicPartition.Topic)
		err := s.InsertMessage(m)
		if err != nil {
			panic(err)
		}
	}
	fmt.Printf(".done, last index=%v", index)
}
