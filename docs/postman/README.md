# Cliente de Pruebas y Suite de Validación con Postman

Este directorio contiene el **cliente oficial de pruebas para Postman** y la suite de validación automatizada para la arquitectura de microservicios de **PDF ExtracText**.

Permite probar y validar de manera interactiva o automatizada (vía **Newman**) todos los endpoints y contratos definidos en `consigna.md`, `openapi.yaml` y los estándares del proyecto (RFC 9457, Twelve-Factor App, anti-TOCTOU).

---

## 📁 Contenido del directorio

| Archivo | Descripción |
|---|---|
| `pdf-extractext.postman_collection.json` | Colección Postman (v2.1.0) con 13 requests organizados en 4 carpetas y 27 assertions automáticas. |
| `pdf-extractext-local.postman_environment.json` | Variables de entorno (`baseUrl: https://pdf-extractext.localhost`, `documentId`, `renamedFilename`). |
| `sample.pdf` | Archivo PDF fixture válido para pruebas de extracción y carga. |
| `sample.txt` | Archivo de texto para verificar el error `415 Unsupported Media Type`. |
| `corrupt.pdf` | Archivo malformado para verificar el error `422 Unprocessable Content`. |

---

## 🚀 Guía de uso rápido

### Opción 1: Ejecución automatizada con Newman (CLI / Headless)

No requiere abrir la interfaz de Postman. Se ejecuta directamente desde la terminal con `npx` (o a través del Makefile):

```bash
# Desde la raíz de pdf-extractext-infrastructure:
make postman
```

O invocando directamente `newman`:

```bash
cd postman
npx --yes newman run pdf-extractext.postman_collection.json \
  -e pdf-extractext-local.postman_environment.json \
  --insecure
```

> **Nota sobre `--insecure`**: necesario porque en entorno local se utiliza TLS con certificados autofirmados / locales generados con `mkcert`.

#### Ejecución alternativa con Docker (sin requerir Node/npm en el host)

```bash
docker run --rm -v "$(pwd):/etc/newman" --network host \
  postman/newman:alpine run pdf-extractext.postman_collection.json \
  -e pdf-extractext-local.postman_environment.json \
  --insecure
```

---

### Opción 2: Uso interactivo en la aplicación de escritorio de Postman

1. **Abrir Postman**.
2. **Importar la Colección y el Entorno**:
   - Clic en el botón **Import** (arriba a la izquierda).
   - Arrastrar o seleccionar:
     - `postman/pdf-extractext.postman_collection.json`
     - `postman/pdf-extractext-local.postman_environment.json`
3. **Seleccionar el Entorno**:
   - En el selector desplegable de entornos (arriba a la derecha), elegir **PDF ExtracText - Local Environment**.
4. **Configuración esencial de certificados SSL**:
   - Al usar `https://pdf-extractext.localhost` con certificados locales, desactivar la validación estricta de SSL en Postman:
     - Ir a **Settings** (ícono de engranaje) -> **General**.
     - Desactivar la opción: **SSL certificate verification** (`OFF`).
5. **Configuración de archivos en Postman**:
   - Para las peticiones `POST /extract` y `POST /upload-pdf`, seleccionar el archivo `sample.pdf` en la solapa **Body** -> `form-data` -> campo `file`.
   - *Tip:* Si configuras el "Working directory" de Postman apuntando a esta carpeta `postman/`, Postman detectará automáticamente `sample.pdf`, `sample.txt` y `corrupt.pdf`.

---

## 🧪 Cobertura de la Suite de Pruebas

La colección está organizada secuencialmente para permitir la ejecución completa de punta a punta con el **Collection Runner** de Postman:

### 1. `01 - Health Check`
- `GET /`: Comprueba que la API esté activa y responda `{"service": "pdf-extractext-api", "status": "UP"}`.

### 2. `02 - Extracción (Stateless - Requerimiento TP)`
- `POST /extract (200 OK)`: Envía `sample.pdf`. Valida el **contrato C1 obligatorio** de `consigna.md`:
  - Retorna JSON con `content` (Markdown no vacío) y `page_count` (entero > 0).
  - No persiste nada en la base de datos.
- `POST /extract - Error 400`: Omite el campo `file`. Valida respuesta de error RFC 9457 (`INVALID_REQUEST`).
- `POST /extract - Error 415`: Envía `sample.txt`. Valida rechazo por tipo no soportado (`UNSUPPORTED_MEDIA_TYPE`).
- `POST /extract - Error 422`: Envía `corrupt.pdf`. Valida error propagado del extractor (`INVALID_PDF_CONTENT`).

### 3. `03 - Orquestación y Persistencia (/upload-pdf)`
- `POST /upload-pdf (201 Created)`: Extrae y persiste el documento en MongoDB (contratos C1 + C2). Captura automáticamente el `id` generado (`pm.environment.set("documentId", jsonData.id)`) para las siguientes peticiones.
- `POST /upload-pdf - Error 409`: Envía el mismo archivo nuevamente. Valida el rechazo por checksum duplicado (barrera anti-TOCTOU garantizada por el índice único de Mongo).

### 4. `04 - Gestión de Documentos (CRUD)`
- `GET /documents`: Verifica que la lista de documentos incluya el `documentId` creado.
- `GET /documents/:id`: Consulta el documento por su ID dinámico.
- `PATCH /documents/:id`: Renombra el documento (`filename: documento_renombrado.pdf`).
- `DELETE /documents/:id`: Elimina el documento (retorna `204 No Content`).
- `GET /documents/:id (Post-eliminación)`: Verifica que ahora retorne `404 Not Found` (RFC 9457 `NOT_FOUND`), dejando la base de datos limpia para la próxima corrida.
- `GET /documents/id-invalido`: Verifica que un ID no hexadecimal de 24 caracteres sea rechazado con `400 Bad Request` en el borde.
