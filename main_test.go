package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLogsToConsoleAndFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{"-log-file", logPath, dir}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("run завершился с кодом %d; stderr: %s", exitCode, stderr.String())
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("не удалось прочитать журнал: %v", err)
	}
	for destination, data := range map[string]string{
		"консоль": stdout.String(),
		"файл":    string(logData),
	} {
		if !strings.Contains(data, "Начало обработки") || !strings.Contains(data, "Итог:") {
			t.Errorf("%s не содержит ожидаемые записи: %q", destination, data)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("неожиданный stderr: %s", stderr.String())
	}
}
