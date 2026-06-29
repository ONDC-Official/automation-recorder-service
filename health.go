package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Service   string    `json:"service"`
}

type healthChecker struct {
	rdb *redis.Client
}

func (hc *healthChecker) handle(w http.ResponseWriter, r *http.Request) {
	status := "healthy"
	if err := hc.rdb.Ping(context.Background()).Err(); err != nil {
		status = "unhealthy"
	}

	response := HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Service:   "automation-recorder-service",
	}

	w.Header().Set("Content-Type", "application/json")
	if status == "unhealthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(response)
}
