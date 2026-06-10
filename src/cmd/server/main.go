package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"nvims-sms/internal/auth"
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

	sessions := auth.NewSessions()
	st := store.New(pool)
	h := handler.New(st, sessions)

	protect := func(fn http.HandlerFunc) http.HandlerFunc {
		return sessions.Middleware(fn).ServeHTTP
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", h.Login)
	mux.HandleFunc("POST /login", h.LoginPost)
	mux.HandleFunc("POST /logout", h.Logout)
	mux.HandleFunc("GET /{$}", protect(h.Menu))
	mux.HandleFunc("GET /register", protect(h.Register))
	mux.HandleFunc("GET /programs", protect(h.Programs))
	mux.HandleFunc("GET /groups", protect(h.Groups))
	mux.HandleFunc("GET /classes", protect(h.Classes))
	mux.HandleFunc("GET /attendance", protect(h.Attendance))
	mux.HandleFunc("GET /attendance/popup", protect(h.AttendancePopup))
	mux.HandleFunc("POST /attendance", protect(h.SetAttendance))
	mux.HandleFunc("GET /results", protect(h.Results))
	mux.HandleFunc("GET /result/popup", protect(h.ResultPopup))
	mux.HandleFunc("POST /result", protect(h.SetResult))
	mux.HandleFunc("POST /result/publish", protect(h.PublishResult))
	mux.HandleFunc("POST /result/publish-sc", protect(h.PublishSCColumn))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	addr := ":8080"
	log.Printf("listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
