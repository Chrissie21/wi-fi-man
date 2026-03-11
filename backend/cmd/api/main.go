package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"wifi-man/backend/internal/config"
	apihttp "wifi-man/backend/internal/http"
	"wifi-man/backend/internal/http/handlers"
	"wifi-man/backend/internal/http/middleware"
	"wifi-man/backend/internal/jobs"
	"wifi-man/backend/internal/repository"
	"wifi-man/backend/internal/service"
)

func main() {
	cfg := config.Load()

	ctx := context.Background()
	var store repository.Store
	var pgStore *repository.PostgresStore
	switch cfg.StoreBackend {
	case "memory":
		store = repository.NewMemoryStore()
		log.Printf("using memory store backend")
	default:
		if err := repository.RunMigrations(cfg.DatabaseDSN, cfg.MigrationsPath); err != nil {
			log.Fatalf("run migrations: %v", err)
		}
		var err error
		pgStore, err = repository.NewPostgresStore(ctx, cfg.DatabaseDSN)
		if err != nil {
			log.Fatalf("init postgres store: %v", err)
		}
		defer pgStore.Close()
		store = pgStore
		log.Printf("using postgres store backend")
	}

	var redisClient *redis.Client
	if cfg.AsynqEnabled || cfg.RateLimitEnabled {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
		})
		if err := redisClient.Ping(ctx).Err(); err != nil {
			log.Fatalf("connect redis: %v", err)
		}
		defer redisClient.Close()
	}

	auditService := service.NewAuditService(store)
	planService := service.NewPlanService(store)
	if err := planService.SeedDefaults(ctx); err != nil {
		log.Fatalf("seed plans: %v", err)
	}

	tokenService := service.NewTokenService(store, auditService)
	quotaService := service.NewQuotaService(store)
	sessionService := service.NewSessionService(store, quotaService)
	paymentService := service.NewPaymentService(store, auditService, cfg.MobileMoneySecret)
	var primaryGatewayController service.GatewayController
	if cfg.RadiusCoAAddr != "" && cfg.RadiusCoASecret != "" {
		primaryGatewayController = service.NewCoAController(cfg.RadiusCoAAddr, cfg.RadiusCoASecret, 3*time.Second)
	}
	var fallbackGatewayController service.GatewayController
	fallbackURL := cfg.RouterOSAPIURL
	fallbackToken := cfg.RouterOSAPIToken
	if fallbackURL == "" {
		fallbackURL = cfg.GatewayDisconnectURL
		fallbackToken = cfg.GatewayAuthToken
	}
	if fallbackURL != "" {
		fallbackGatewayController = service.NewHTTPGatewayController(fallbackURL, fallbackToken)
	}
	gatewayController := service.NewFallbackGatewayController(primaryGatewayController, fallbackGatewayController)
	gatewayService := service.NewGatewayIntegrationService(store, gatewayController)
	tokenService.SetGatewayIntegration(gatewayService)

	var producer *jobs.Producer
	if cfg.AsynqEnabled {
		producer = jobs.NewProducer(cfg.RedisAddr, cfg.RedisPassword)
		defer producer.Close()
		gatewayService.SetRetryQueue(producer)
	}

	portalHandler := handlers.NewPortalHandler(tokenService, planService, paymentService, cfg.PortalTitle)
	adminHandler := handlers.NewAdminHandler(tokenService, sessionService, paymentService, planService)
	accountingHandler := handlers.NewAccountingHandler(sessionService, gatewayService)
	paymentHandler := handlers.NewPaymentHandler(paymentService)
	gatewayHandler := handlers.NewGatewayHandler(gatewayService)
	healthHandler := handlers.NewHealthHandler()

	var portalLimiter gin.HandlerFunc
	var webhookLimiter gin.HandlerFunc
	if cfg.RateLimitEnabled && redisClient != nil {
		window := time.Duration(cfg.RateLimitWindowSecs) * time.Second
		portalLimiter = middleware.RateLimit(redisClient, cfg.RateLimitRequests, window)
		webhookLimiter = middleware.RateLimit(redisClient, cfg.RateLimitRequests, window)
	}

	router := apihttp.NewRouter(apihttp.Dependencies{
		Cfg:                cfg,
		Portal:             portalHandler,
		Admin:              adminHandler,
		Accounting:         accountingHandler,
		Payment:            paymentHandler,
		Gateway:            gatewayHandler,
		Health:             healthHandler,
		PortalRateLimit:    portalLimiter,
		WebhookRateLimiter: webhookLimiter,
	})

	httpServer := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 20 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	var worker *jobs.Worker
	var scheduler *jobs.Scheduler
	if cfg.AsynqEnabled {
		worker = jobs.NewWorker(cfg.RedisAddr, cfg.RedisPassword, cfg.AsynqConcurrency, tokenService, gatewayService)
		worker.Start()
		var err error
		scheduler, err = jobs.NewScheduler(cfg.RedisAddr, cfg.RedisPassword, cfg.AsynqSweepCron)
		if err != nil {
			log.Fatalf("init asynq scheduler: %v", err)
		}
		scheduler.Start()
	}

	var radiusServer *service.RadiusServer
	if cfg.RadiusEnabled && cfg.RadiusSecret != "" {
		radiusService := service.NewRadiusService(tokenService, sessionService, gatewayService)
		var err error
		radiusServer, err = service.NewRadiusServer(cfg.RadiusAuthAddr, cfg.RadiusAcctAddr, cfg.RadiusSecret, radiusService)
		if err != nil {
			log.Fatalf("init radius server: %v", err)
		}
		radiusServer.Start(context.Background())
		log.Printf("radius auth listening on %s, accounting on %s", cfg.RadiusAuthAddr, cfg.RadiusAcctAddr)
	}

	go func() {
		log.Printf("api listening on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown failed: %v", err)
	}
	if worker != nil {
		worker.Stop()
	}
	if scheduler != nil {
		scheduler.Stop()
	}
	if radiusServer != nil {
		radiusServer.Stop()
	}
}
