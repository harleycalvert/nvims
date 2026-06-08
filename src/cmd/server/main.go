package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"nvims-sms/internal/handler"
	"nvims-sms/internal/store"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://nvims:jjnhbFC56RDWRTJHBjhb98uibe@localhost:5432/nvims-sms"
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("ping: %v", err)
	}
	log.Println("database connected")

	st := store.New(pool)
	h := handler.New(st)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.Index)
	mux.HandleFunc("GET /programs", h.Programs)
	mux.HandleFunc("GET /classes", h.Classes)
	mux.HandleFunc("GET /attendance", h.Attendance)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	addr := ":8080"
	log.Printf("listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
