package db

import (
	"fmt"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func BuildPostgressDSN(cfg config.PostgresConfig) string {
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=%s",
		cfg.Host,
		cfg.User,
		cfg.Password,
		cfg.DBName,
		cfg.Port,
		cfg.SSLMode,
		cfg.TimeZone,
	)
}

func ConnectPostgres(cfg config.PostgresConfig) (*gorm.DB, error) {
	dsn := BuildPostgressDSN(cfg)

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("connect postgres failed: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("connect sql db failed: %w", err)
	}

	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err = sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping sql db failed: %w", err)
	}

	return gormDB, nil
}
