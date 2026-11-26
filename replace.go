package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func replaceFileSafely(originalPath, replacementPath string) error {
	backup, err := os.CreateTemp(filepath.Dir(originalPath), ".compress-files-backup-*")
	if err != nil {
		return fmt.Errorf("не удалось подготовить резервное имя: %w", err)
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		os.Remove(backupPath)
		return fmt.Errorf("не удалось закрыть резервный файл: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("не удалось подготовить резервный файл: %w", err)
	}

	if err := os.Rename(originalPath, backupPath); err != nil {
		return fmt.Errorf("не удалось переименовать оригинал перед заменой: %w", err)
	}
	if err := os.Rename(replacementPath, originalPath); err != nil {
		if rollbackErr := os.Rename(backupPath, originalPath); rollbackErr != nil {
			return fmt.Errorf("не удалось установить сжатый файл: %v; ВНИМАНИЕ: оригинал сохранён в %q, но не восстановлен автоматически: %w", err, backupPath, rollbackErr)
		}
		return fmt.Errorf("не удалось установить сжатый файл, оригинал восстановлен: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("сжатый файл установлен, но не удалось удалить резервную копию %q: %w", backupPath, err)
	}
	return nil
}
