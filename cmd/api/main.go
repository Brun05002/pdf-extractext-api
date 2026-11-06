package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/api"
	"github.com/pdf-extractext/api/internal/consul"
	"github.com/pdf-extractext/api/internal/core"
	"github.com/pdf-extractext/api/internal/services"
)

// version y commit se inyectan con -ldflags en la imagen; quedan en el Meta de Consul.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	// Logging estructurado JSON por defecto para todo el proceso.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := core.LoadConfig()
	if err != nil {
		slog.Error("configuración inválida", "error", err)
		os.Exit(1)
	}

	if cfg.ConsulEnabled {
		loadConsulKV(cfg)
	}

	app := fiber.New(fiber.Config{
		BodyLimit: int(cfg.MaxFileSizeMB) * 1024 * 1024,
		// Traducción global de errores a problem+json (RFC 9457).
		ErrorHandler: api.CustomErrorHandler,
		// WriteTimeout > timeout del cliente de extracción (30s) para no cortar /extract.
		ReadTimeout:  35 * time.Second,
		WriteTimeout: 40 * time.Second,
		IdleTimeout:  120 * time.Second,
	})

	app.Use(core.RequestID())
	app.Use(core.Logger())
	api.RegisterRoot(app)

	// Timeout estricto: backpressure contra un upstream lento.
	extraction := services.NewHTTPExtractionClient(cfg.ExtractionURL, cfg.ExtractionHost, 30*time.Second)
	persistence := services.NewHTTPPersistenceClient(cfg.PersistenceURL, 10*time.Second)

	api.RegisterExtract(app, extraction, cfg.MaxFileSizeMB)
	api.RegisterUploadPDF(app, extraction, persistence)
	api.RegisterDocuments(app, persistence)

	// SIGINT/SIGTERM: drenar requests en vuelo antes de salir (PID 1 del contenedor).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Se binda el listener antes de registrarse para que el health check no falle al arrancar.
	ln, err := net.Listen("tcp", cfg.Port)
	if err != nil {
		slog.Error("no se pudo abrir el puerto", "puerto", cfg.Port, "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("iniciando servidor", "puerto", cfg.Port)
		if err := app.Listener(ln); err != nil {
			slog.Error("el servidor terminó con error", "error", err)
			os.Exit(1)
		}
	}()

	var consulClient *consul.Client
	var reg consul.Registration
	registered := false
	if cfg.ConsulEnabled {
		consulClient, reg, registered = registerInConsul(ctx, cfg)
	}

	<-ctx.Done()
	slog.Info("señal de apagado recibida; deregistrando y drenando conexiones")
	if registered {
		if err := consul.Shutdown(consulClient, reg.ID, 5*time.Second, app); err != nil {
			slog.Error("error durante el apagado", "error", err)
			os.Exit(1)
		}
	} else if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		// 10s < stop_grace_period del compose (15s): sin esto Docker envía SIGKILL.
		slog.Error("error durante el apagado", "error", err)
		os.Exit(1)
	}
	slog.Info("servidor detenido correctamente")
}

// loadConsulKV aplica la configuración remota con fail-open: un error no impide arrancar.
func loadConsulKV(cfg *core.Config) {
	client := consul.NewClient(cfg.ConsulAddr, cfg.ConsulToken)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	values, err := consul.LoadConfigKV(ctx, client)
	if err != nil {
		slog.Warn("no se pudo leer Consul KV; se usan variables de entorno o defaults", "error", err)
		return
	}
	if err := core.ApplyKV(cfg, values); err != nil {
		slog.Warn("configuración de Consul KV inválida; se usan variables de entorno o defaults", "error", err)
	}
}

// registerInConsul resuelve la IP de la red services y lanza el alta con reintentos.
func registerInConsul(ctx context.Context, cfg *core.Config) (*consul.Client, consul.Registration, bool) {
	client := consul.NewClient(cfg.ConsulAddr, cfg.ConsulToken)

	address, err := consul.DetectLocalIP(cfg.ConsulAddr)
	if err != nil {
		slog.Warn("no se pudo detectar la IP para el registro en Consul", "error", err)
		return client, consul.Registration{}, false
	}

	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}

	reg := consul.BuildRegistration(consul.RegistrationOptions{
		Hostname:   hostname,
		Address:    address,
		Port:       cfg.ServicePort,
		HealthPath: cfg.ServiceHealthPath,
		Name:       cfg.ServiceName,
		Version:    version,
		Commit:     commit,
	})
	go consul.RegisterWithRetry(ctx, client, reg, time.Second, 30*time.Second)
	return client, reg, true
}
