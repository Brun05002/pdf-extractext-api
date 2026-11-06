package consul

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultServiceName = "pdf-extractext-api"
	defaultServicePort = 8000
	defaultHealthPath  = "/"
)

// Registration es el payload exacto de /v1/agent/service/register.
type Registration struct {
	ID      string            `json:"ID"`
	Name    string            `json:"Name"`
	Address string            `json:"Address"`
	Port    int               `json:"Port"`
	Tags    []string          `json:"Tags"`
	Meta    map[string]string `json:"Meta"`
	Check   AgentCheck        `json:"Check"`
}

// AgentCheck define el health check HTTP que Consul ejecuta contra la instancia.
type AgentCheck struct {
	HTTP                           string `json:"HTTP"`
	Interval                       string `json:"Interval"`
	Timeout                        string `json:"Timeout"`
	DeregisterCriticalServiceAfter string `json:"DeregisterCriticalServiceAfter"`
}

// RegistrationOptions agrupa la identidad y los metadatos de build.
type RegistrationOptions struct {
	Hostname   string
	Address    string
	Port       int
	HealthPath string
	Name       string
	Version    string
	Commit     string
}

// Registrar abstrae el registro para poder testear el ciclo de vida sin Consul.
type Registrar interface {
	Register(ctx context.Context, payload any) error
	Deregister(ctx context.Context, id string) error
}

// Shutdowner abstrae el apagado del servidor Fiber.
type Shutdowner interface {
	ShutdownWithTimeout(timeout time.Duration) error
}

// BuildRegistration arma el payload con Tags funcionales para Traefik v3.5.
func BuildRegistration(opts RegistrationOptions) Registration {
	port := opts.Port
	if port == 0 {
		port = defaultServicePort
	}
	name := opts.Name
	if name == "" {
		name = defaultServiceName
	}
	healthPath := opts.HealthPath
	if healthPath == "" {
		healthPath = defaultHealthPath
	}
	if !strings.HasPrefix(healthPath, "/") {
		healthPath = "/" + healthPath
	}

	portValue := strconv.Itoa(port)
	ruleValue := "Host(`pdf-extractext.localhost`)"
	traefikKeys := map[string]string{
		"traefik.enable":                                     "true",
		"traefik.http.routers.api.rule":                      ruleValue,
		"traefik.http.routers.api.entrypoints":               "https",
		"traefik.http.routers.api.tls":                       "true",
		"traefik.http.services.api.loadbalancer.server.port": portValue,
	}

	tags := []string{
		"traefik.enable=true",
		"traefik.http.routers.api.rule=" + ruleValue,
		"traefik.http.routers.api.entrypoints=https",
		"traefik.http.routers.api.tls=true",
		"traefik.http.services.api.loadbalancer.server.port=" + portValue,
	}

	meta := make(map[string]string, len(traefikKeys)+2)
	for key, value := range traefikKeys {
		meta[key] = value
	}
	meta["version"] = opts.Version
	meta["commit"] = opts.Commit

	return Registration{
		ID:      "api-" + opts.Hostname,
		Name:    name,
		Address: opts.Address,
		Port:    port,
		Tags:    tags,
		Meta:    meta,
		Check: AgentCheck{
			HTTP:                           fmt.Sprintf("http://%s:%d%s", opts.Address, port, healthPath),
			Interval:                       "10s",
			Timeout:                        "2s",
			DeregisterCriticalServiceAfter: "1m",
		},
	}
}

// DetectLocalIP descubre la IP real del contenedor usando la ruta hacia Consul.
func DetectLocalIP(consulAddr string) (string, error) {
	return detectLocalIP(consulAddr, net.Dial)
}

// detectLocalIP abre un socket UDP: no envía tráfico y la ruta elige la IP de la red services.
func detectLocalIP(consulAddr string, dial func(network, address string) (net.Conn, error)) (string, error) {
	host, err := consulHostPort(consulAddr)
	if err != nil {
		return "", err
	}

	conn, err := dial("udp", host)
	if err != nil {
		return "", fmt.Errorf("detectar IP local: %w", err)
	}
	defer conn.Close()

	udpAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || udpAddr.IP == nil {
		return "", fmt.Errorf("detectar IP local: dirección UDP inesperada")
	}
	if udpAddr.IP.IsLoopback() || udpAddr.IP.IsUnspecified() {
		return "", fmt.Errorf("detectar IP local: se obtuvo una dirección loopback (%s)", udpAddr.IP)
	}
	return udpAddr.IP.String(), nil
}

func consulHostPort(consulAddr string) (string, error) {
	raw := consulAddr
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("CONSUL_HTTP_ADDR inválido: %w", err)
		}
		raw = parsed.Host
	}
	if raw == "" {
		return "", fmt.Errorf("CONSUL_HTTP_ADDR vacío")
	}
	if !strings.Contains(raw, ":") {
		raw += ":8500"
	}
	return raw, nil
}

// RegisterWithRetry registra en segundo plano con backoff exponencial acotado.
func RegisterWithRetry(ctx context.Context, registrar Registrar, reg Registration, initial, max time.Duration) {
	_ = registerWithRetry(ctx, registrar, reg, initial, max, time.Sleep)
}

func registerWithRetry(ctx context.Context, registrar Registrar, reg Registration, initial, max time.Duration, sleep func(time.Duration)) error {
	if initial <= 0 {
		initial = time.Second
	}
	if max < initial {
		max = initial
	}

	backoff := initial
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := registrar.Register(ctx, reg); err == nil {
			slog.Info("servicio registrado en Consul", "id", reg.ID)
			return nil
		} else {
			slog.Warn("registro en Consul falló; reintentando", "id", reg.ID, "backoff", backoff, "error", err)
		}
		sleep(backoff)
		if backoff *= 2; backoff > max {
			backoff = max
		}
	}
}

// Shutdown deregistra antes de drenar: Traefik deja de enviar tráfico antes del cierre.
func Shutdown(registrar Registrar, serviceID string, drain time.Duration, app Shutdowner) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := registrar.Deregister(ctx, serviceID); err != nil {
		slog.Warn("no se pudo deregistrar de Consul", "id", serviceID, "error", err)
	}
	if drain > 0 {
		time.Sleep(drain)
	}
	return app.ShutdownWithTimeout(10 * time.Second)
}
