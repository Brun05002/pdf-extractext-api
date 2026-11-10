package api

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"

	"github.com/gofiber/fiber/v2"
)

var errBodyTooLarge = errors.New("request body exceeds limit")

// pdfInput expone el stream de la parte "file" sin materializar el PDF completo.
type pdfInput struct {
	Filename string
	Reader   io.Reader

	closeBody func() error
	limit     *bodyLimitReader
}

func (p *pdfInput) Read(b []byte) (int, error) {
	return p.Reader.Read(b)
}

// Close libera el body stream sin drenar el resto de un upload abortado.
func (p *pdfInput) Close() error {
	if p.closeBody != nil {
		return p.closeBody()
	}
	return nil
}

// LimitExceeded indica si el body superó MAX_FILE_SIZE_MB durante el streaming.
func (p *pdfInput) LimitExceeded() bool {
	return p.limit != nil && p.limit.exceeded
}

// openPDFInput parsea el multipart en streaming y devuelve únicamente la parte "file".
func openPDFInput(c *fiber.Ctx, maxBytes int64) (*pdfInput, error) {
	contentLength := int64(c.Request().Header.ContentLength())
	if contentLength > maxBytes {
		_ = c.Request().CloseBodyStream()
		return nil, requestTooLargeError()
	}

	mediaType, params, err := mime.ParseMediaType(c.Get(fiber.HeaderContentType))
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		_ = c.Request().CloseBodyStream()
		return nil, fiber.NewError(fiber.StatusBadRequest, "el cuerpo debe ser multipart/form-data con boundary")
	}

	stream := c.Request().BodyStream()
	if stream == nil {
		stream = bytes.NewReader(c.Body())
	}

	limit := &bodyLimitReader{reader: stream, remaining: maxBytes}
	reader := multipart.NewReader(limit, params["boundary"])

	success := false
	defer func() {
		if !success {
			_ = c.Request().CloseBodyStream()
		}
	}()

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, fiber.NewError(fiber.StatusBadRequest, "se requiere el campo multipart 'file'")
		}
		if err != nil {
			if limit.exceeded || errors.Is(err, errBodyTooLarge) {
				return nil, requestTooLargeError()
			}
			return nil, fiber.NewError(fiber.StatusBadRequest, "multipart/form-data inválido")
		}

		if part.FormName() != "file" || part.FileName() == "" {
			_ = part.Close()
			continue
		}

		filename := part.FileName()
		if !isPDF(filename) {
			return nil, fiber.NewError(fiber.StatusUnsupportedMediaType, "solo se aceptan archivos PDF")
		}

		// Valida los magic bytes para rechazar binarios no-PDF antes del streaming.
		magic := make([]byte, 4)
		n, err := io.ReadFull(part, magic)
		if err != nil || n < 4 || !bytes.Equal(magic[:4], []byte("%PDF")) {
			return nil, fiber.NewError(fiber.StatusUnsupportedMediaType, "solo se aceptan archivos PDF")
		}

		success = true
		return &pdfInput{
			Filename:  filename,
			Reader:    io.MultiReader(bytes.NewReader(magic[:n]), part),
			closeBody: c.Request().CloseBodyStream,
			limit:     limit,
		}, nil
	}
}

func requestTooLargeError() error {
	return fiber.NewError(fiber.StatusRequestEntityTooLarge, "el cuerpo supera MAX_FILE_SIZE_MB")
}

// bodyLimitReader corta el stream en cuanto se supera el máximo, incluso con chunked.
type bodyLimitReader struct {
	reader    io.Reader
	remaining int64
	exceeded  bool
}

func (r *bodyLimitReader) Read(p []byte) (int, error) {
	if r.exceeded {
		return 0, errBodyTooLarge
	}
	if r.remaining < 0 {
		r.exceeded = true
		return 0, errBodyTooLarge
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining < int64(len(p)) {
		p = p[:r.remaining+1]
	}

	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	if r.remaining < 0 {
		r.exceeded = true
		return n, errBodyTooLarge
	}
	return n, err
}
