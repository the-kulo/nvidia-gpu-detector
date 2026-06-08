package main

import (
	"fmt"
	"log"

	"github.com/the-kulo/nvidia-gpu-detector/internal/center"
	"github.com/the-kulo/nvidia-gpu-detector/internal/config"
	"github.com/the-kulo/nvidia-gpu-detector/internal/db"
	"github.com/the-kulo/nvidia-gpu-detector/internal/model"
	"github.com/the-kulo/nvidia-gpu-detector/internal/store"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	gormDB, err := db.ConnectPostgres(cfg.Database.Postgres)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("postgres connected:", gormDB != nil)

	err = gormDB.AutoMigrate(&model.Agent{})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("database migrated")

	agentStore := store.NewAgentStore(gormDB)

	server := center.NewServer(agentStore)

	addr := cfg.Server.Addr
	fmt.Println("server addr:", addr)

	err = server.StartServer(addr)
	if err != nil {
		log.Fatal(err)
	}
}
