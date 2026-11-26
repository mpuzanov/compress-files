package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type statistics struct {
	eligible    int
	compressed  int
	notSmaller  int
	failed      int
	bytesBefore int64
	bytesAfter  int64
}

type compressionResult struct {
	before int64
	after  int64
}

type processor struct {
	opts   options
	logger *log.Logger
}

func newProcessor(opts options, logger *log.Logger) *processor {
	return &processor{opts: opts, logger: logger}
}

func (p *processor) processPath(path string) (statistics, error) {
	info, err := os.Stat(path)
	if err != nil {
		return statistics{}, fmt.Errorf("не удалось открыть путь %q: %w", path, err)
	}
	if info.IsDir() {
		return p.processDirectory(path)
	}
	if !info.Mode().IsRegular() {
		return statistics{}, fmt.Errorf("путь не является обычным файлом: %q", path)
	}
	kind := supportedKind(path)
	if kind == "" {
		return statistics{}, fmt.Errorf("неподдерживаемое расширение файла %q; разрешены .pdf, .jpg и .jpeg", path)
	}
	if info.Size() <= p.opts.minSize {
		p.logger.Printf("ПРОПУЩЕН %s: размер %s не превышает порог %s", path,
			formatBytes(info.Size()), formatBytes(p.opts.minSize))
		return statistics{}, nil
	}

	var stats statistics
	p.processEligibleFile(path, kind, info, &stats)
	return stats, nil
}

func (p *processor) processDirectory(root string) (statistics, error) {
	var stats statistics
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			stats.failed++
			p.logger.Printf("ОШИБКА %s: %v", path, walkErr)
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if !p.opts.recursive && path != root {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() && entry.Type() != 0 {
			return nil
		}
		kind := supportedKind(path)
		if kind == "" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			stats.failed++
			p.logger.Printf("ОШИБКА %s: не удалось прочитать сведения о файле: %v", path, err)
			return nil
		}
		if !info.Mode().IsRegular() || info.Size() <= p.opts.minSize {
			return nil
		}

		p.processEligibleFile(path, kind, info, &stats)
		return nil
	})
	return stats, err
}

func (p *processor) processEligibleFile(path, kind string, info os.FileInfo, stats *statistics) {
	stats.eligible++
	result, changed, err := p.processFile(path, kind, info)
	if err != nil {
		stats.failed++
		p.logger.Printf("ОШИБКА %s: %v", path, err)
		return
	}
	if !changed {
		stats.notSmaller++
		p.logger.Printf("БЕЗ ИЗМЕНЕНИЙ %s: сжатая версия не меньше оригинала", path)
		return
	}

	stats.compressed++
	stats.bytesBefore += result.before
	stats.bytesAfter += result.after
	action := "СЖАТ"
	if p.opts.dryRun {
		action = "МОЖНО СЖАТЬ"
	}
	p.logger.Printf("%s %s: %s -> %s, экономия %s", action, path,
		formatBytes(result.before), formatBytes(result.after), formatBytes(result.before-result.after))
}

func (p *processor) processFile(path, kind string, originalInfo os.FileInfo) (compressionResult, bool, error) {
	temp, err := os.CreateTemp(filepath.Dir(path), ".compress-files-*")
	if err != nil {
		return compressionResult{}, false, fmt.Errorf("не удалось создать временный файл: %w", err)
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		os.Remove(tempPath)
		return compressionResult{}, false, fmt.Errorf("не удалось закрыть временный файл: %w", err)
	}
	defer os.Remove(tempPath)

	switch kind {
	case "jpeg":
		err = compressJPEG(path, tempPath, p.opts.jpegQuality)
	case "pdf":
		err = compressPDF(path, tempPath, p.opts.pdfQuality, p.opts.ghostscriptPath)
	default:
		err = fmt.Errorf("неподдерживаемый тип %q", kind)
	}
	if err != nil {
		return compressionResult{}, false, err
	}

	compressedInfo, err := os.Stat(tempPath)
	if err != nil {
		return compressionResult{}, false, fmt.Errorf("не удалось проверить сжатый файл: %w", err)
	}
	result := compressionResult{before: originalInfo.Size(), after: compressedInfo.Size()}
	if compressedInfo.Size() <= 0 {
		return result, false, errors.New("получен пустой файл")
	}
	if compressedInfo.Size() >= originalInfo.Size() {
		return result, false, nil
	}
	if p.opts.dryRun {
		return result, true, nil
	}

	if err := os.Chmod(tempPath, originalInfo.Mode().Perm()); err != nil {
		return result, false, fmt.Errorf("не удалось сохранить права доступа: %w", err)
	}
	if err := replaceFileSafely(path, tempPath); err != nil {
		return result, false, err
	}
	return result, true, nil
}

func supportedKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "jpeg"
	case ".pdf":
		return "pdf"
	default:
		return ""
	}
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d Б", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit && exp < 3; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cБ", float64(size)/float64(div), []rune("КМГТ")[exp])
}
