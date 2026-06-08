package main

import (
	"fmt"
	"log"

	"github.com/the-kulo/nvidia-gpu-detector/internal/center"
	"github.com/the-kulo/nvidia-gpu-detector/internal/config"
	"github.com/the-kulo/nvidia-gpu-detector/internal/db"
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

	addr := cfg.Server.Addr
	fmt.Println("server addr:", addr)

	err = center.StartServer(addr)
	if err != nil {
		log.Fatal(err)
	}
}
