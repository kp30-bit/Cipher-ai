package main

import (
	"concall-analyser/config"
	"concall-analyser/internal/controller"
	"concall-analyser/internal/db"
	"concall-analyser/internal/interfaces"
	"concall-analyser/internal/repository/mongo"
	"concall-analyser/internal/service/analytics"
	"concall-analyser/internal/usecase"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
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

	// Initialize analytics service (shared between usecase and middleware)
	analyticsRepo := mongo.NewAnalyticsRepository(db)
	analyticsService := analytics.NewAnalyticsService(analyticsRepo)

	usecaseInstance, err := usecase.NewConcallFetcher(db, cfg, analyticsService)
	if err != nil {
		log.Fatalf("❌ Failed to create usecase: %v", err)
	}
	router := gin.Default()

	// Enable CORS for API routes
	router.Use(corsMiddleware())

	// Register API routes (prefixed with /api)
	controller.RegisterRoutes(router, usecaseInstance, analyticsService)

	// Serve static frontend assets (JS, CSS, images, etc.)
	router.Static("/static", "./frontend/build/static")

	// Serve other static files (favicon, manifest, etc.)
	router.StaticFile("/favicon.ico", "./frontend/build/favicon.ico")
	router.StaticFile("/manifest.json", "./frontend/build/manifest.json")
	router.StaticFile("/robots.txt", "./frontend/build/robots.txt")

	// Serve React app for all non-API routes (SPA routing)
	router.NoRoute(func(c *gin.Context) {
		// Don't serve index.html for API routes
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
			return
		}
		c.File("./frontend/build/index.html")
	})

	return &App{
		Router:      router,
		Usecase:     usecaseInstance,
		MongoClient: client,
		Config:      cfg,
	}
}

// CORS middleware
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
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

	ctx, cancel := context.WithTimeout(context.Background(), 3600*time.Second)
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
