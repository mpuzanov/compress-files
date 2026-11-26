package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const defaultMinSize int64 = 1024 * 1024

type options struct {
	inputPath       string
	minSize         int64
	jpegQuality     int
	pdfQuality      string
	ghostscriptPath string
	recursive       bool
	dryRun          bool
	logFile         string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compress-files", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var opts options
	fs.Int64Var(&opts.minSize, "min-size", defaultMinSize, "минимальный размер файла в байтах")
	fs.IntVar(&opts.jpegQuality, "jpeg-quality", 75, "качество JPEG от 1 до 100")
	fs.StringVar(&opts.pdfQuality, "pdf-quality", "ebook", "профиль PDF: screen, ebook, printer, prepress, default")
	fs.StringVar(&opts.ghostscriptPath, "ghostscript", "", "путь к Ghostscript (gswin64c.exe или gs)")
	fs.BoolVar(&opts.recursive, "recursive", true, "искать файлы во вложенных каталогах")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "проверить файлы без замены оригиналов")
	fs.StringVar(&opts.logFile, "log-file", defaultLogPath(), "путь к файлу журнала")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Использование: %s [параметры] <файл или каталог>\n\n", filepath.Base(os.Args[0]))
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	opts.inputPath = fs.Arg(0)

	logFile, err := os.OpenFile(opts.logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "Ошибка: не удалось открыть файл журнала %q: %v\n", opts.logFile, err)
		return 2
	}
	defer logFile.Close()
	logger := log.New(io.MultiWriter(stdout, logFile), "", log.Ldate|log.Ltime)

	if err := validateOptions(&opts); err != nil {
		logger.Println("ОШИБКА:", err)
		return 2
	}

	logger.Printf("Начало обработки пути %s; порог %s; JPEG %d; PDF %s; dry-run=%t",
		opts.inputPath, formatBytes(opts.minSize), opts.jpegQuality, opts.pdfQuality, opts.dryRun)
	logger.Printf("Файл журнала: %s", opts.logFile)
	processor := newProcessor(opts, logger)
	stats, err := processor.processPath(opts.inputPath)
	printSummary(logger, stats, opts.dryRun)
	if err != nil {
		logger.Println("ОШИБКА:", err)
		return 1
	}
	if stats.failed > 0 {
		return 1
	}
	return 0
}

func validateOptions(opts *options) error {
	info, err := os.Stat(opts.inputPath)
	if err != nil {
		return fmt.Errorf("не удалось открыть путь %q: %w", opts.inputPath, err)
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return fmt.Errorf("указанный путь не является обычным файлом или каталогом: %q", opts.inputPath)
	}
	abs, err := filepath.Abs(opts.inputPath)
	if err != nil {
		return fmt.Errorf("не удалось определить полный путь: %w", err)
	}
	opts.inputPath = abs

	if opts.minSize < 0 {
		return errors.New("параметр -min-size не может быть отрицательным")
	}
	if opts.jpegQuality < 1 || opts.jpegQuality > 100 {
		return errors.New("параметр -jpeg-quality должен быть от 1 до 100")
	}
	validPDFQuality := map[string]bool{
		"screen": true, "ebook": true, "printer": true, "prepress": true, "default": true,
	}
	opts.pdfQuality = strings.ToLower(opts.pdfQuality)
	if !validPDFQuality[opts.pdfQuality] {
		return errors.New("параметр -pdf-quality должен быть: screen, ebook, printer, prepress или default")
	}
	return nil
}

func printSummary(logger *log.Logger, stats statistics, dryRun bool) {
	mode := "заменено"
	if dryRun {
		mode = "могло быть заменено"
	}
	logger.Println()
	logger.Printf("Итог: найдено подходящих файлов: %d; %s: %d; без уменьшения: %d; ошибок: %d",
		stats.eligible, mode, stats.compressed, stats.notSmaller, stats.failed)
	if stats.compressed > 0 {
		logger.Printf("Экономия: %s (с %s до %s)",
			formatBytes(stats.bytesBefore-stats.bytesAfter), formatBytes(stats.bytesBefore), formatBytes(stats.bytesAfter))
	}
}

func defaultLogPath() string {
	executable, err := os.Executable()
	if err != nil {
		return "compress-files.log"
	}
	return filepath.Join(filepath.Dir(executable), "compress-files.log")
}
