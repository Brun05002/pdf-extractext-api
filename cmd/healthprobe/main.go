// healthprobe es un mini-binario para la imagen distroless (sin shell, sin
// wget ni curl). Hace GET a la raíz del servicio y traduce el resultado a
// código de salida: 0 si responde 200, 1 en cualquier otro caso.
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	os.Exit(run())
}

func run() int {
	port := strings.TrimPrefix(strings.TrimSpace(os.Getenv("PORT")), ":")
	if port == "" {
		port = "8000"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	//nolint:gosec // URL local fija; solo el puerto viene de PORT.
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/")
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthprobe: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthprobe: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
