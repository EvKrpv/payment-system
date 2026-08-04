package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	connStr := "postgres://postgres:postgres@127.0.0.1:5432/payment_db?sslmode=disable"
	fmt.Println("🔄 Connecting to:", connStr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		return
	}
	defer conn.Close(context.Background())

	fmt.Println("✅ Connected successfully!")

	var result int
	err = conn.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		fmt.Printf("❌ Query error: %v\n", err)
		return
	}
	fmt.Printf("✅ Query result: %d\n", result)
}
