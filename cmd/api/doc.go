// Package main es el entrypoint del microservicio API Gateway para el
// procesamiento de PDFs dentro del Proyecto Cabras. Orquesta la extracción
// de texto (microservicio de extracción) y el CRUD de metadatos
// (microservicio de persistencia), exponiendo un contrato HTTP homogéneo
// con errores traducidos y trazabilidad vía X-Request-ID.
package main
