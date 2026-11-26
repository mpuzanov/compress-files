package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSupportedKind(t *testing.T) {
	tests := map[string]string{
		"file.JPG":  "jpeg",
		"file.jpeg": "jpeg",
		"file.PdF":  "pdf",
		"file.png":  "",
	}
	for name, want := range tests {
		if got := supportedKind(name); got != want {
			t.Errorf("supportedKind(%q) = %q, нужно %q", name, got, want)
		}
	}
}

func TestCompressJPEGCreatesValidSmallerFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.jpg")
	target := filepath.Join(dir, "target.jpg")
	writeNoisyJPEG(t, source, 1200, 1200, 100)
	createEmptyFile(t, target)

	if err := compressJPEG(source, target, 55); err != nil {
		t.Fatalf("compressJPEG: %v", err)
	}
	sourceInfo, _ := os.Stat(source)
	targetInfo, _ := os.Stat(target)
	if targetInfo.Size() >= sourceInfo.Size() {
		t.Fatalf("сжатый файл не меньше: исходный %d, сжатый %d", sourceInfo.Size(), targetInfo.Size())
	}
	file, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := jpeg.Decode(file); err != nil {
		t.Fatalf("результат не является корректным JPEG: %v", err)
	}
}

func TestCompressJPEGPreservesEXIF(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.jpg")
	target := filepath.Join(dir, "target.jpg")
	writeNoisyJPEG(t, source, 300, 300, 95)
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	exifSegment := []byte{0xff, 0xe1, 0x00, 0x0d, 'E', 'x', 'i', 'f', 0, 0, 't', 'e', 's', 't', 0}
	withEXIF := append([]byte{}, original[:2]...)
	withEXIF = append(withEXIF, exifSegment...)
	withEXIF = append(withEXIF, original[2:]...)
	if err := os.WriteFile(source, withEXIF, 0o600); err != nil {
		t.Fatal(err)
	}
	createEmptyFile(t, target)

	if err := compressJPEG(source, target, 60); err != nil {
		t.Fatalf("compressJPEG: %v", err)
	}
	compressed, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(compressed, exifSegment) {
		t.Fatal("EXIF-сегмент не сохранён")
	}
}

func TestProcessorReplacesEligibleJPEG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.jpg")
	writeNoisyJPEG(t, path, 900, 900, 100)
	before, _ := os.Stat(path)

	var output bytes.Buffer
	opts := options{inputPath: dir, minSize: 1000, jpegQuality: 50, pdfQuality: "ebook", recursive: true}
	stats, err := newProcessor(opts, log.New(&output, "", 0)).processDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if stats.compressed != 1 || stats.failed != 0 {
		t.Fatalf("неожиданная статистика: %+v; вывод: %s", stats, output.String())
	}
	if after.Size() >= before.Size() {
		t.Fatalf("файл не уменьшился: было %d, стало %d", before.Size(), after.Size())
	}
	if backups, _ := filepath.Glob(filepath.Join(dir, ".compress-files-*")); len(backups) != 0 {
		t.Fatalf("остались временные файлы: %v", backups)
	}
}

func TestProcessorHandlesOnlySpecifiedFile(t *testing.T) {
	dir := t.TempDir()
	selected := filepath.Join(dir, "selected.jpg")
	other := filepath.Join(dir, "other.jpg")
	writeNoisyJPEG(t, selected, 700, 700, 100)
	writeNoisyJPEG(t, other, 700, 700, 100)
	otherBefore, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}

	opts := options{inputPath: selected, minSize: 1000, jpegQuality: 45, pdfQuality: "ebook", recursive: true}
	stats, err := newProcessor(opts, log.New(&bytes.Buffer{}, "", 0)).processPath(selected)
	if err != nil {
		t.Fatal(err)
	}
	otherAfter, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	if stats.eligible != 1 || stats.compressed != 1 || stats.failed != 0 {
		t.Fatalf("неожиданная статистика: %+v", stats)
	}
	if !bytes.Equal(otherBefore, otherAfter) {
		t.Fatal("файл, не указанный во входном пути, был изменён")
	}
}

func TestProcessorUsesStrictSizeThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exact.jpg")
	writeNoisyJPEG(t, path, 100, 100, 90)
	info, _ := os.Stat(path)

	stats, err := newProcessor(options{minSize: info.Size(), jpegQuality: 50, recursive: true}, log.New(&bytes.Buffer{}, "", 0)).processDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if stats.eligible != 0 {
		t.Fatalf("файл размером ровно в порог не должен обрабатываться: %+v", stats)
	}
}

func TestDryRunDoesNotReplaceFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.jpg")
	writeNoisyJPEG(t, path, 700, 700, 100)
	before, _ := os.ReadFile(path)

	stats, err := newProcessor(options{minSize: 1000, jpegQuality: 40, recursive: true, dryRun: true}, log.New(&bytes.Buffer{}, "", 0)).processDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if stats.compressed != 1 || !bytes.Equal(before, after) {
		t.Fatalf("dry-run изменил оригинал или дал неверную статистику: %+v", stats)
	}
}

func TestValidatePDF(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.pdf")
	if err := os.WriteFile(valid, []byte("%PDF-1.4\nbody\n%%EOF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validatePDF(valid); err != nil {
		t.Fatalf("корректный PDF отклонён: %v", err)
	}
	invalid := filepath.Join(dir, "invalid.pdf")
	if err := os.WriteFile(invalid, []byte("not a pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validatePDF(invalid); err == nil || !strings.Contains(err.Error(), "заголовок") {
		t.Fatalf("ожидалась ошибка заголовка PDF, получено: %v", err)
	}
}

func writeNoisyJPEG(t *testing.T, path string, width, height, quality int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x*31 + y*17) % 256),
				G: uint8((x*13 + y*47) % 256),
				B: uint8((x*53 + y*7) % 256),
				A: 255,
			})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: quality}); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func createEmptyFile(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
