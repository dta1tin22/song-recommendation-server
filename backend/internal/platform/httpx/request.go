package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// maxRequestBodyBytes caps request bodies. Auth payloads are tiny; anything
// larger is a mistake or an attack.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

// DecodeJSON reads exactly one JSON object from the body, rejecting unknown
// fields and oversized payloads.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return err
	}

	// A second value in the body means the client sent something we did not expect.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("body must contain a single JSON object")
	}

	return nil
}

// WriteDecodeError maps JSON decoding failures onto client errors without
// echoing server internals. The message names the request's shape problem, since
// a client that sends an unknown field or a bad type deserves a usable hint.
func WriteDecodeError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		WriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body is too large")
		return
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	switch {
	case errors.Is(err, io.EOF):
		WriteError(w, http.StatusBadRequest, "invalid_body", "request body is empty")
	case errors.Is(err, io.ErrUnexpectedEOF):
		WriteError(w, http.StatusBadRequest, "invalid_body", "request body contains truncated JSON")
	case errors.As(err, &syntaxErr):
		WriteError(w, http.StatusBadRequest, "invalid_body", "request body contains malformed JSON")
	case errors.As(err, &typeErr):
		WriteError(w, http.StatusBadRequest, "invalid_body", fmt.Sprintf("field %q has the wrong type", typeErr.Field))
	default:
		WriteError(w, http.StatusBadRequest, "invalid_body", err.Error())
	}
}
