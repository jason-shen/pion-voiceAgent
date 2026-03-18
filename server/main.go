package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/voiceagent/server/internal/config"
	"github.com/voiceagent/server/internal/room"
	"github.com/voiceagent/server/internal/signaling"
	"github.com/voiceagent/server/internal/tts"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	if cfg.DeepgramAPIKey == "" {
		log.Fatal("DEEPGRAM_API_KEY must be set (required for STT)")
	}
	if cfg.OpenAIAPIKey == "" {
		log.Fatal("OPENAI_API_KEY must be set")
	}
	if _, err := tts.NewClient(cfg); err != nil {
		log.Fatalf("TTS config: %v (set TTS_PROVIDER=cartesia with CARTESIA_API_KEY, or TTS_PROVIDER=deepgram)", err)
	}

	rm := room.NewManager(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/whip/", signaling.NewWHIPHandler(rm))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	handler := corsMiddleware(mux)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: handler,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("Voice agent server listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rm.CloseAll()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	log.Println("Server stopped")
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Expose-Headers", "Location, ETag")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
