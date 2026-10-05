# pdf-extractext-api

Microservicio **API de borde** del sistema pdf-extractext. Recibe archivos PDF
por HTTP, aplica los límites del borde (tamaño máximo del cuerpo, validación de
formato) y delega la extracción de texto en `pdf-extractext-extractor`. La
orquestación completa (extracción + persistencia en un solo endpoint público)
está planificada como issue #7 — ver [Roadmap](#roadmap).

- **Stack:** Go 1.27 + Fiber v2 (fasthttp). Streaming 100% en memoria: cero
  volcados a disco en el hot path.
- **Estado:** bootstrap en desarrollo (TDD). No desplegable como pipeline
  completo todavía — ver [Limitaciones](#limitaciones-actuales).

## Arquitectura

```
Traefik (:80)                        red `services`
   │
   ▼
pdf-extractext-api (:8000)
   ├──► extraction  (:8001)          extracción de texto (Python/PyMuPDF)
   └──► persistence (:8002) ──► MongoDB (:27017)   red `data`
```

El orquestador completo de contenedores (compose, redes, smoke checks) vive en
el repo hermano `pdf-extractext-infrastructure`. Esta API todavía no está
declarada en ese compose.

## Variables de entorno

Se validan al arranque con fail-fast (`log.Fatalf`): si falta una obligatoria,
el proceso no levanta.

| Variable | Requerida | Default | Descripción |
|---|---|---|---|
| `PORT` | No | `:8000` | Puerto de escucha. **Formato `net/http`: con dos puntos iniciales** |
| `EXTRACTION_URL` | **Sí** | — | URL base del microservicio de extracción. Ej.: `http://extraction:8001` |
| `PERSISTENCE_URL` | **Sí** | — | URL base del microservicio de persistencia. Ej.: `http://persistence:8002`. Obligatoria al arranque, **aún sin consumo** (issue #6) |
| `MAX_FILE_SIZE_MB` | No | `20` | Límite duro del cuerpo HTTP en MB, aplicado vía `BodyLimit`. Cuenta el multipart COMPLETO (fronteras + headers + archivo) |

## Endpoints

### `GET /`

Probe informal (única ruta viva en el binario actual).

```json
{ "service": "pdf-extractext-api", "status": "ok" }
```

### `POST /extract`

Vivo en el binario. Recibe el PDF y lo reenvía al extractor como
`multipart/form-data` (contrato C1, campo `file`) streameando en memoria.

- **Request:** `multipart/form-data`, campo `file`, solo extensión `.pdf`.
- **Response 200 (contrato TP):**

```json
{ "content": "<texto extraído>", "page_count": 42 }
```

| Status | Causa |
|---|---|
| 400 | Falta el campo multipart `file` |
| 413 | El cuerpo supera `MAX_FILE_SIZE_MB` (rechazado por `BodyLimit`) |
| 415 | El archivo no es un PDF |
| 502 | Fallo del upstream de extracción |

El formato de error actual (`{"error": "..."}`, en español) es **provisional**:
la traducción/estandarización global de errores llega con la issue #8.

## Limitaciones actuales

Registro honesto de deuda técnica; cada ítem referencia la issue que lo
resuelve.

1. **`PERSISTENCE_URL` es requisito muerto** — obligatoria al arranque pero
   ningún código la consume. → issue #6 (cliente de persistencia).
2. **`X-Request-ID` sin explotar** — el middleware lo genera/propaga en la
   respuesta, pero no se loguea ni se reenvía al upstream. → junto con #7/#8.
3. **Validación de PDF solo por extensión** — no hay chequeo de magic bytes
   (`%PDF-`). → endurecimiento posterior.
4. **Sin endpoints de lectura/escritura** de documentos persistidos. → issues
   #10 (GET) y #11 (PATCH / DELETE).
5. **Sin logging estructurado, métricas ni tracing.** → issue #13.
## Roadmap

| Issue | Descripción | Estado |
|---|---|---|
| #4 | Renombrar módulo Go a nombre definitivo | Pendiente |
| #5 | Integración Continua (CI) y automatización | Pendiente |
| #6 | Cliente de persistencia (CRUD HTTP hacia la base) | Pendiente |
| #7 | Orquestador `POST /upload-pdf` (combina extraction + persistence) | Pendiente |
| #8 | Traducción global de errores | Pendiente |
| #9 | Documentar especificación OpenAPI | Pendiente |
| #10 | Endpoints de lectura (GET) | Pendiente |
| #11 | Endpoints de escritura (PATCH / DELETE) | Pendiente |
| #12 | Docker multi-stage para Go (endurecimiento de la base) | Base hecha, refino pendiente |
| #13 | Housekeeping y observabilidad mínima | Pendiente |

## Desarrollo

Requisitos: Go 1.27+.

```bash
# Tests: 15 escritos, todos en verde.
go test ./...

# Ejecución local (falla sin las obligatorias, por diseño)
EXTRACTION_URL=http://localhost:8001 \
PERSISTENCE_URL=http://localhost:8002 \
go run ./cmd/api
```

### Docker (base)

El `Dockerfile` es una **base bootstrap**: multi-stage con build estático
(`CGO_ENABLED=0`, `-trimpath`), usuario no-root y `HEALTHCHECK` sobre `GET /`.
El endurecimiento completo es ownership de la issue #12.

```bash
docker build -t pdf-extractext-api:bootstrap .

docker run --rm -p 8000:8000 \
  -e EXTRACTION_URL=http://extraction:8001 \
  -e PERSISTENCE_URL=http://persistence:8002 \
  pdf-extractext-api:bootstrap
```

Dentro de las redes del compose de `pdf-extractext-infrastructure`, los hosts
`extraction` y `persistence` resuelven por DNS de Docker.

## Estructura

```
pdf-extractext-api/
├── cmd/
│   └── api/
    │       └── main.go            # entrypoint: config + GET / (wiring de /extract pendiente, #7)
└── internal/
    ├── api/
    │   ├── root.go            # GET /
    │   └── extract.go         # handler POST /extract (no registrado aún)
    ├── core/
    │   ├── config.go          # env vars con validación fail-fast
    │   └── middleware.go      # X-Request-ID
    └── services/
        └── extraction_client.go  # cliente HTTP hacia el extractor (streaming)
```
