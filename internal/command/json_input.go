package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ViceMe-AI/cli/internal/output"
)

const maxCommandJSONBytes = 8 << 20

func readJSONObject(filename, code string) (json.RawMessage, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, output.Validation(code, "could not open the JSON input file").WithCause(err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCommandJSONBytes+1))
	if err != nil {
		return nil, output.Validation(code, "could not read the JSON input file").WithCause(err)
	}
	if len(data) > maxCommandJSONBytes {
		return nil, output.Validation(code, "JSON input exceeds the 8 MiB limit")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, output.Validation(code, "input must be one JSON object")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return nil, output.Validation(code, "input must be one valid JSON object").WithCause(err)
	}
	if object == nil {
		return nil, output.Validation(code, "input must be one JSON object")
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, output.Internal("JSON_INPUT_ENCODE_FAILED", "could not normalize the JSON input", err)
	}
	return canonical, nil
}

func rawJSONObject(fields map[string]any) (json.RawMessage, error) {
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("encoded request is empty")
	}
	return data, nil
}
