package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func compressPDF(sourcePath, targetPath, quality, configuredGhostscript string) error {
	ghostscript, err := findGhostscript(configuredGhostscript)
	if err != nil {
		return err
	}

	args := []string{
		"-sDEVICE=pdfwrite",
		"-dCompatibilityLevel=1.4",
		"-dPDFSETTINGS=/" + quality,
		"-dNOPAUSE",
		"-dBATCH",
		"-dQUIET",
		"-dSAFER",
		"-dDetectDuplicateImages=true",
		"-dCompressFonts=true",
		"-dSubsetFonts=true",
		"-sOutputFile=" + targetPath,
		sourcePath,
	}
	output, err := exec.Command(ghostscript, args...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("Ghostscript не смог сжать PDF: %w; %s", err, message)
		}
		return fmt.Errorf("Ghostscript не смог сжать PDF: %w", err)
	}
	if err := validatePDF(targetPath); err != nil {
		return err
	}
	return nil
}

func findGhostscript(configured string) (string, error) {
	if configured != "" {
		info, err := os.Stat(configured)
		if err != nil {
			return "", fmt.Errorf("Ghostscript не найден по пути %q: %w", configured, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("путь Ghostscript указывает на каталог: %q", configured)
		}
		return configured, nil
	}

	for _, name := range []string{"gswin64c.exe", "gswin32c.exe", "gs"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}

	var matches []string
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if base == "" {
			continue
		}
		found, _ := filepath.Glob(filepath.Join(base, "gs", "gs*", "bin", "gswin64c.exe"))
		matches = append(matches, found...)
		found, _ = filepath.Glob(filepath.Join(base, "gs", "gs*", "bin", "gswin32c.exe"))
		matches = append(matches, found...)
	}
	if len(matches) > 0 {
		sort.Strings(matches)
		return matches[len(matches)-1], nil
	}
	return "", fmt.Errorf("Ghostscript не найден; установите его или укажите путь параметром -ghostscript")
}

func validatePDF(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("не удалось проверить сжатый PDF: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("не удалось получить размер сжатого PDF: %w", err)
	}
	if info.Size() < 5 {
		return fmt.Errorf("проверка сжатого PDF не пройдена: отсутствует заголовок PDF")
	}
	header := make([]byte, 5)
	if _, err := io.ReadFull(file, header); err != nil {
		return fmt.Errorf("не удалось прочитать заголовок сжатого PDF: %w", err)
	}
	if !bytes.Equal(header, []byte("%PDF-")) {
		return fmt.Errorf("проверка сжатого PDF не пройдена: отсутствует заголовок PDF")
	}
	tailSize := min(int64(2048), info.Size())
	if _, err := file.Seek(-tailSize, io.SeekEnd); err != nil {
		return fmt.Errorf("не удалось прочитать конец сжатого PDF: %w", err)
	}
	tail := make([]byte, tailSize)
	if _, err := io.ReadFull(file, tail); err != nil {
		return fmt.Errorf("не удалось прочитать конец сжатого PDF: %w", err)
	}
	if !bytes.Contains(tail, []byte("%%EOF")) {
		return fmt.Errorf("проверка сжатого PDF не пройдена: отсутствует маркер конца PDF")
	}
	return nil
}
