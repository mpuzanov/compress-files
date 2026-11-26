package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"image/jpeg"
	"io"
	"os"
)

func compressJPEG(sourcePath, targetPath string, quality int) (err error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("не удалось открыть JPEG: %w", err)
	}
	imageData, err := jpeg.Decode(source)
	closeErr := source.Close()
	if err != nil {
		return fmt.Errorf("не удалось декодировать JPEG: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("не удалось закрыть JPEG: %w", closeErr)
	}
	metadata, err := readJPEGMetadata(sourcePath)
	if err != nil {
		return fmt.Errorf("не удалось прочитать метаданные JPEG: %w", err)
	}

	target, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return fmt.Errorf("не удалось создать сжатый JPEG: %w", err)
	}
	writer := bufio.NewWriter(target)
	if _, err := writer.Write([]byte{0xff, 0xd8}); err != nil {
		target.Close()
		return fmt.Errorf("не удалось записать заголовок сжатого JPEG: %w", err)
	}
	for _, segment := range metadata {
		if _, err := writer.Write(segment); err != nil {
			target.Close()
			return fmt.Errorf("не удалось сохранить метаданные JPEG: %w", err)
		}
	}
	// jpeg.Encode сам пишет SOI (Start Of Image), поэтому отбрасываем его через skipSOIWriter.
	if err := jpeg.Encode(&skipSOIWriter{writer: writer}, imageData, &jpeg.Options{Quality: quality}); err != nil {
		target.Close()
		return fmt.Errorf("не удалось записать сжатый JPEG: %w", err)
	}
	if err := writer.Flush(); err != nil {
		target.Close()
		return fmt.Errorf("не удалось завершить запись сжатого JPEG: %w", err)
	}
	if err := target.Close(); err != nil {
		return fmt.Errorf("не удалось закрыть сжатый JPEG: %w", err)
	}

	check, err := os.Open(targetPath)
	if err != nil {
		return fmt.Errorf("не удалось проверить сжатый JPEG: %w", err)
	}
	_, err = jpeg.DecodeConfig(check)
	closeErr = check.Close()
	if err != nil {
		return fmt.Errorf("проверка сжатого JPEG не пройдена: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("не удалось закрыть проверяемый JPEG: %w", closeErr)
	}
	return nil
}

// skipSOIWriter удаляет первые два байта FF D8 из потока jpeg.Encode.
type skipSOIWriter struct {
	writer  io.Writer
	skipped int
}

func (w *skipSOIWriter) Write(data []byte) (int, error) {
	originalLength := len(data)
	if w.skipped < 2 {
		need := 2 - w.skipped
		if len(data) <= need {
			w.skipped += len(data)
			return originalLength, nil
		}
		data = data[need:]
		w.skipped = 2
	}
	if _, err := w.writer.Write(data); err != nil {
		return 0, err
	}
	return originalLength, nil
}

// readJPEGMetadata сохраняет EXIF (APP1) и цветовой профиль ICC (APP2).
func readJPEGMetadata(path string) ([][]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)

	soi := make([]byte, 2)
	if _, err := io.ReadFull(reader, soi); err != nil {
		return nil, err
	}
	if soi[0] != 0xff || soi[1] != 0xd8 {
		return nil, fmt.Errorf("отсутствует маркер SOI")
	}

	var result [][]byte
	for {
		prefix, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if prefix != 0xff {
			return nil, fmt.Errorf("неожиданный байт до данных изображения: %02x", prefix)
		}
		marker, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		for marker == 0xff {
			marker, err = reader.ReadByte()
			if err != nil {
				return nil, err
			}
		}
		if marker == 0xda || marker == 0xd9 { // SOS или EOI
			return result, nil
		}
		if marker == 0x01 || marker >= 0xd0 && marker <= 0xd8 {
			continue
		}

		var lengthBytes [2]byte
		if _, err := io.ReadFull(reader, lengthBytes[:]); err != nil {
			return nil, err
		}
		length := int(binary.BigEndian.Uint16(lengthBytes[:]))
		if length < 2 {
			return nil, fmt.Errorf("некорректная длина сегмента %02x", marker)
		}
		payload := make([]byte, length-2)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return nil, err
		}
		if marker == 0xe1 || marker == 0xe2 {
			segment := make([]byte, 0, length+2)
			segment = append(segment, 0xff, marker, lengthBytes[0], lengthBytes[1])
			segment = append(segment, payload...)
			result = append(result, segment)
		}
	}
}
