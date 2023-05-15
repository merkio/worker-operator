package config

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/kelseyhightower/envconfig"
)

const (
	App = "worker-controller"
)

var ControllerConfig ControllerConf
var KafkaConfig KafkaConf

type ControllerConf struct {
	Namespace      string `default:"system" envconfig:"NAMESPACE"`
	SelectorLabel  string `default:"app" envconfig:"SELECTOR_LABEL"`
	SelectorValue  string `default:"worker" envconfig:"SELECTOR_VALUE"`
	FinalizerValue string `default:"kubernetes" envconfig:"FINALIZER_VALUE"`
}

type KafkaConf struct {
	BootstrapServers string `default:"kafka-service:9092" envconfig:"KAFKA_BOOTSTRAP_SERVERS"`
	Topic            string `default:"worker-status" envconfig:"KAFKA_TOPIC"`
}

func LoadConfig(log logr.Logger) {

	var controllerConf ControllerConf
	var kafkaConf KafkaConf

	process(log, &controllerConf, App)
	process(log, &kafkaConf, App)

	ControllerConfig = controllerConf
	KafkaConfig = kafkaConf
}

// process load config and merge with env variables
func process(log logr.Logger, conf interface{}, app string) {
	confType := fmt.Sprintf("%T", conf)
	l := log.WithName(confType)

	err := envconfig.Process("", conf)

	if err != nil {
		l.Error(err, "Configuration could not be processed")
		panic(err)
	}
	l.Info("Configuration processed")
}
