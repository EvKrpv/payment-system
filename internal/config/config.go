package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	ServerPort string
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
}

func Load() (Config, error) {
	serverPort := os.Getenv("SERVER_PORT")
	dbHost := os.Getenv("DB_HOST")
	dbPort := getEnvAsInt("DB_PORT", 5432)
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	if serverPort == "" {
		serverPort = "8081"
	}
	if dbHost == "" {
		return Config{}, errors.New("DB_HOST environment variable is required")
	}
	if dbUser == "" {
		return Config{}, errors.New("DB_USER environment variable is required")
	}
	if dbPassword == "" {
		return Config{}, errors.New("DB_PASSWORD environment variable is required")
	}
	if dbName == "" {
		return Config{}, errors.New("DB_NAME environment variable is required")
	}

	return Config{
		ServerPort: serverPort,
		DBHost:     dbHost,
		DBPort:     dbPort,
		DBUser:     dbUser,
		DBPassword: dbPassword,
		DBName:     dbName,
	}, nil
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
