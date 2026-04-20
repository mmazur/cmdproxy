package protocol

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
)

type Request struct {
	Cmd  string   `json:"cmd"`
	Args []string `json:"args"`
}

type Response struct {
	ExitCode int    `json:"exit_code"`
	Stdout   []byte `json:"stdout,omitempty"`
	Stderr   []byte `json:"stderr,omitempty"`
	Error    string `json:"error,omitempty"`
}

func Encode(r Request) (string, error) {
	j, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(j); err != nil {
		return "", fmt.Errorf("gzip write: %w", err)
	}
	if err := gz.Close(); err != nil {
		return "", fmt.Errorf("gzip close: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func Decode(s string) (Request, error) {
	compressed, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return Request{}, fmt.Errorf("base64 decode: %w", err)
	}

	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return Request{}, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	j, err := io.ReadAll(gz)
	if err != nil {
		return Request{}, fmt.Errorf("gzip read: %w", err)
	}

	var r Request
	if err := json.Unmarshal(j, &r); err != nil {
		return Request{}, fmt.Errorf("unmarshal request: %w", err)
	}
	return r, nil
}
