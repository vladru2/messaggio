package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	"github.com/caarlos0/env/v8"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	_ "github.com/joho/godotenv/autoload"
)

type Config struct {
	Kafka                  string `env:"KAFKA,notEmpty"`
	BackendDSN             string `env:"BACKEND_DB_DSN,notEmpty"`
	BackendMaxOpenConn     int    `env:"BACKEND_DB_MAX_OPEN_CONN,notEmpty"`
	BackendMaxIdleConn     int    `env:"BACKEND_DB_MAX_IDLE_CONN,notEmpty"`
	BackendConnIdleSeconds int    `env:"BACKEND_DB_CONN_IDLE_SECONDS,notEmpty"`
}

// Message Сообщение
type Message struct {
	Uuid      uuid.UUID `gorm:"not null;type:uuid;primaryKey" json:"uuid"`
	UserName  string    `gorm:"not null;type:VARCHAR(128)" json:"user_name"`
	Time      time.Time `gorm:"not null;type:timestamptz;default:now()" json:"time"`
	Text      string    `gorm:"not null" json:"text"`
	Processed bool      `gorm:"not null;default:false" json:"processed"`
}

// Stat Статистика обработанных сообщений по нику
type Stat struct {
	UserName       string `gorm:"not null;type:VARCHAR(128);primaryKey" json:"user_name"`
	ProcessedCount int    `gorm:"not null;default:0" json:"processed_count"`
}

func main() {
	gracefulShutdown := make(chan os.Signal, 1)
	signal.Notify(gracefulShutdown, syscall.SIGTERM, syscall.SIGINT)

	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("invalid config: %s", err)
	}

	backendDB := psqlOpen(
		cfg.BackendDSN,
		cfg.BackendConnIdleSeconds,
		cfg.BackendMaxOpenConn,
		cfg.BackendMaxIdleConn,
	)

	consumer, err := sarama.NewConsumer([]string{cfg.Kafka}, nil)
	if err != nil {
		log.Fatalf("Failed to create consumer: %v", err)
	} else {
		log.Println("Started Kafka Consumer!")
	}
	defer consumer.Close()

	partitions, _ := consumer.Partitions("messages")

	partConsumer, err := consumer.ConsumePartition("messages", partitions[0], sarama.OffsetNewest)
	if err != nil {
		log.Fatalf("Failed to consume partition: %v", err)
	}
	defer partConsumer.Close()

	// Горутина для обработки входящих сообщений от Kafka
	go func() {
		for {
			consumedMsg, ok := <-partConsumer.Messages()
			if !ok {
				log.Println("Channel closed, exiting goroutine")
				return
			}

			var msg Message
			err := json.Unmarshal(consumedMsg.Value, &msg)
			if err != nil {
				fmt.Println(err)
				break
			}

			res := backendDB.Model(msg).Update("Processed", true)
			if res.Error != nil {
				fmt.Println(res.Error)
				break
			}

			res = backendDB.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_name"}},
				DoUpdates: clause.Assignments(map[string]interface{}{"processed_count": gorm.Expr("stats.processed_count + ?", 1)}),
			}).Create(&Stat{
				UserName:       msg.UserName,
				ProcessedCount: 1,
			})
			if res.Error != nil {
				fmt.Println(res.Error)
				break
			}
		}
	}()

	<-gracefulShutdown

	log.Println("gracefully shutting down")
}

func psqlOpen(
	PsqlDSN string,
	PsqlConnIdleSeconds int,
	PsqlMaxOpenConn int,
	PsqlMaxIdleConn int,
) *gorm.DB {
	gormDB, err := gorm.Open(postgres.Open(PsqlDSN))
	if err != nil {
		panic(err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		panic(err)
	}
	sqlDB.SetConnMaxLifetime(time.Second * time.Duration(PsqlConnIdleSeconds))
	sqlDB.SetMaxOpenConns(PsqlMaxOpenConn)
	sqlDB.SetMaxIdleConns(PsqlMaxIdleConn)
	return gormDB
}
