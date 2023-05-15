package kafka

import (
	"context"
	"fmt"
	"strings"

	"encoding/json"

	"github.com/google/uuid"

	"github.com/go-logr/logr"
	"github.com/merkio/worker-operator/internal/config"
	kafka "github.com/segmentio/kafka-go"
)

type Message struct {
	Status   string `json:"status"`
	WorkerID string `json:"worker_id"`
	WorkerIP string `json:"worker_ip"`
}

func (m *Message) convert() (kafka.Message, error) {
	value, err := json.Marshal(m)
	if err != nil {
		return kafka.Message{}, err
	}

	return kafka.Message{
		Key:   []byte(uuid.NewString()),
		Value: value,
	}, nil
}

type KafkaProducer struct {
	writer kafka.Writer
	log    logr.Logger
}

func (p *KafkaProducer) PublishMessage(message Message) {
	m, err := message.convert()
	if err != nil {
		p.log.Error(err, "failed to convert message")
	}
	p.log.Info(fmt.Sprintf("Publish message: \n Key: %v \nValue: %v", string(m.Key), message))
	e := p.writer.WriteMessages(context.Background(), m)
	if e != nil {
		p.log.Error(e, "failed publish message")
	}
}

func NewProducer(config config.KafkaConf, log logr.Logger) *KafkaProducer {
	return &KafkaProducer{
		writer: kafka.Writer{
			Addr:     kafka.TCP(strings.Split(config.BootstrapServers, ",")...),
			Topic:    config.Topic,
			Balancer: &kafka.RoundRobin{},
		},
		log: log,
	}
}
