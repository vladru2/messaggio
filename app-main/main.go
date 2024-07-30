package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	"github.com/caarlos0/env/v8"
	"github.com/labstack/echo/v4"
	"golang.org/x/net/http2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	_ "github.com/joho/godotenv/autoload"
	"github.com/labstack/echo/v4/middleware"
)

type Config struct {
	Kafka string `env:"KAFKA,notEmpty"`

	HttpListenNetwork string `env:"HTTP_LISTEN_NETWORK,notEmpty"`
	HttpListenAddress string `env:"HTTP_LISTEN_ADDRESS,notEmpty"`

	BackendDSN             string `env:"BACKEND_DB_DSN,notEmpty"`
	BackendMaxOpenConn     int    `env:"BACKEND_DB_MAX_OPEN_CONN,notEmpty"`
	BackendMaxIdleConn     int    `env:"BACKEND_DB_MAX_IDLE_CONN,notEmpty"`
	BackendConnIdleSeconds int    `env:"BACKEND_DB_CONN_IDLE_SECONDS,notEmpty"`
}

func main() {
	gracefulShutdown := make(chan os.Signal, 1)
	signal.Notify(gracefulShutdown, syscall.SIGTERM, syscall.SIGINT)

	ctx := context.Background()

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

	httpListener, err := net.Listen(cfg.HttpListenNetwork, cfg.HttpListenAddress)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = httpListener.Close() }()

	webApp := echo.New()
	webApp.Use(middleware.Recover())
	webApp.Use(middleware.Logger())

	kafkaProducer, err := sarama.NewAsyncProducer([]string{cfg.Kafka}, nil)
	if err != nil {
		panic(err)
	} else {
		log.Println("Started Kafka Producer!")
	}

	newService(&ctx, &cfg, backendDB, webApp.Group("/api"), kafkaProducer)

	go func() {
		s := &http2.Server{
			MaxConcurrentStreams: 100,
			MaxReadFrameSize:     1048576,
			IdleTimeout:          30 * time.Second,
		}
		webApp.Listener = httpListener
		if err := webApp.StartH2CServer("", s); err != nil && err != http.ErrServerClosed {
			log.Panicf("http: %s\n", err)
		}
	}()

	<-gracefulShutdown

	log.Println("gracefully shutting down")

	if err := webApp.Shutdown(context.Background()); err != nil {
		log.Panicf("http shutdown failed: %+v\n", err)
	}
	log.Println("http shut down")
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
