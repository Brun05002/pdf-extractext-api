// healthprobe es un mini-binario para la imagen distroless (sin shell, sin
// wget ni curl). Hace GET a la raíz del servicio y traduce el resultado a
// código de salida: 0 si responde 200, 1 en cualquier otro caso.
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = ":8000"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1" + port + "/")
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthprobe: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthprobe: status %d\n", resp.StatusCode)
		os.Exit(1)
	}
}
