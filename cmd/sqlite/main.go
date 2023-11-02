package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	_ "github.com/mattn/go-sqlite3"
)

var (
	max          = 1000
	monobroker   = "localhost:9092"
	multibrokers = "localhost:9092,localhost:9093"
	brokers      = multibrokers
)

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
