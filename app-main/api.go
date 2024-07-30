package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

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

type Service struct {
	ctx           *context.Context
	cfg           *Config
	db            *gorm.DB
	asyncProducer sarama.AsyncProducer
}

func newService(
	ctx *context.Context,
	cfg *Config,
	db *gorm.DB,
	group *echo.Group,
	asyncProducer sarama.AsyncProducer,
) *Service {
	err := db.AutoMigrate(
		&Message{},
		&Stat{},
	)
	if err != nil {
		panic(err)
	}

	s := Service{ctx: ctx, cfg: cfg, db: db, asyncProducer: asyncProducer}

	//Прием сообщения
	group.POST("/message", s.recieveMessage)

	//Статистика
	group.GET("/stats", s.getStats)

	return &s
}

type MessageReq struct {
	UserName string `json:"userName"`
	Text     string `json:"text"`
}

// Прием сообщения
func (s *Service) recieveMessage(c echo.Context) error {
	var req MessageReq
	err := c.Bind(&req)
	if err != nil {
		fmt.Println(err)
		return c.NoContent(http.StatusBadRequest)
	}

	msg := Message{
		Uuid:      uuid.New(),
		UserName:  req.UserName,
		Time:      time.Now(),
		Text:      req.Text,
		Processed: false,
	}

	res := s.db.Create(&msg)
	if res.Error != nil {
		fmt.Println(res.Error)
		return c.NoContent(http.StatusInternalServerError)
	}

	jsonMsg, err := json.Marshal(&msg)
	if err != nil {
		fmt.Println(res.Error)
		return c.NoContent(http.StatusInternalServerError)
	}

	kafkaMessage := &sarama.ProducerMessage{
		Topic: "messages",
		Value: sarama.StringEncoder(jsonMsg),
	}

	s.asyncProducer.Input() <- kafkaMessage

	return c.JSON(http.StatusOK, msg.Uuid)
}

type responce struct {
	TotalProcessed   int    `json:"total_processed"`
	ProcessedPerUser []Stat `json:"processed_per_users"`
}

// Статистика
func (s *Service) getStats(c echo.Context) error {
	var total int
	var perUser []Stat

	res := s.db.Find(&perUser)
	if res.Error != nil {
		fmt.Println(res.Error)
		return c.NoContent(http.StatusInternalServerError)
	}

	res = s.db.Model(&Stat{}).Select("sum(processed_count)").Find(&total)
	if res.Error != nil {
		fmt.Println(res.Error)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.JSON(http.StatusOK, &responce{
		TotalProcessed:   total,
		ProcessedPerUser: perUser,
	})
}
