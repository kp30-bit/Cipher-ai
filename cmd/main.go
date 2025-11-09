package main

import (
	"concall-analyser/config"
	"concall-analyser/internal/controller"
	"concall-analyser/internal/db"
	"concall-analyser/internal/interfaces"
	"concall-analyser/internal/usecase"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

// Singletons
var (
	cfgInstance *config.Config
	cfgOnce     sync.Once
	mongoClient *db.MongoClient
	mongoDB     *db.MongoDB
	mongoOnce   sync.Once
)

// App struct
type App struct {
	Router      *gin.Engine
	Usecase     interfaces.Usecase
	MongoClient *db.MongoClient
	Config      *config.Config
}

func main() {
	app := NewApp()
	srv := &http.Server{
		Addr:    ":" + app.Config.Port,
		Handler: app.Router,
	}
	go func() {
		log.Printf("🚀 Server is running on port %s", app.Config.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ Server error: %v", err)
		}
	}()

	// Graceful shutdown
	gracefulShutdown(srv, mongoClient)

}

func NewApp() *App {
	cfg := GetConfig()

	client, db := GetMongo(cfg)
	usecase := usecase.NewConcallFetcher(db, cfg)
	router := gin.Default()

	// Register routes
	controller.RegisterRoutes(router, usecase)

	// Serve static frontend assets (JS, CSS)
	// router.Static("/static", "./frontend")

	// // Serve main HTML file
	// router.LoadHTMLFiles("frontend/index.html")

	// Render frontend when visiting root
	// router.GET("/", func(c *gin.Context) {
	// 	c.HTML(http.StatusOK, "index.html", nil)
	// })

	return &App{
		Router:      router,
		Usecase:     usecase,
		MongoClient: client,
		Config:      cfg,
	}
}

func GetConfig() *config.Config {
	cfgOnce.Do(func() {
		c, err := config.LoadConfig()
		if err != nil {
			log.Fatalf("❌ Failed to load configuration: %v", err)
		}
		cfgInstance = c
		log.Println("✅ Configuration loaded successfully")
	})
	return cfgInstance
}

// GetMongo singleton
func GetMongo(cfg *config.Config) (*db.MongoClient, *db.MongoDB) {
	mongoOnce.Do(func() {

		client, db, err := db.InitMongo(cfg.MongoURI, cfg.MongoDBName)
		if err != nil {
			log.Fatalf("❌ Failed to initialize MongoDB: %v", err)
		}
		mongoClient = client
		mongoDB = db
		log.Println("✅ Connected to MongoDB successfully")
	})
	return mongoClient, mongoDB
}

func gracefulShutdown(srv *http.Server, client *db.MongoClient) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	// Shutdown HTTP server first
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("❌ Server forced to shutdown: %v", err)
	}
	log.Println("✅ HTTP server stopped")

	// Disconnect MongoDB
	if err := client.Disconnect(ctx); err != nil {
		log.Fatalf("❌ Error disconnecting MongoDB: %v", err)
	}
	log.Println("✅ MongoDB connection closed")
}
