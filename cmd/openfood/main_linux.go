package main

import (
	"context"
	"log"
	"net/http"
	"openfood/internal/app"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		client := http.Client{Timeout: 3 * time.Second}
		r, e := client.Get("http://127.0.0.1:8080/readyz")
		if e != nil {
			os.Exit(1)
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL ausente")
	}
	a, e := app.Open(ctx, dsn, os.Getenv("SETUP_TOKEN"), nil)
	if e != nil {
		log.Fatal("database_or_migration_failed")
	}
	defer a.DB.Close()
	a.OnShutdown = cancel
	go a.Worker(ctx)
	s := &http.Server{Addr: ":8080", Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		stop, c := context.WithTimeout(context.Background(), 30*time.Second)
		defer c()
		s.Shutdown(stop)
	}()
	log.Print("OpenFood pronto em :8080; publique somente em loopback")
	if e = s.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal("http_server_failed")
	}
}
