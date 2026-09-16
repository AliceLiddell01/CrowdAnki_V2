package export

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	// ErrPathTraversal сигнализирует о попытке выхода за пределы разрешенного каталога.
	ErrPathTraversal = errors.New("обнаружена попытка выхода за пределы каталога (path traversal)")
)

// ValidateMediaFilename проверяет безопасность относительного имени медиафайла.
func ValidateMediaFilename(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("%w: пустое имя медиафайла", ErrPathTraversal)
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "\\") {
		return fmt.Errorf("%w: имя файла не должно быть абсолютным (%s)", ErrPathTraversal, name)
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || strings.Contains(cleaned, "/../") || strings.Contains(cleaned, "\\..\\") {
		return fmt.Errorf("%w: недопустимый относительный путь (%s)", ErrPathTraversal, name)
	}
	return nil
}

// CopyMediaFiles копирует разрешенные уникальные медиафайлы из mediaDir в destMediaDir,
// рассчитывая их размеры и SHA-256 хэши за один проход чтения.
func CopyMediaFiles(mediaDir string, destMediaDir string, fileNames []string) ([]MediaItem, []string, int64, time.Duration, error) {
	startTime := time.Now()

	// Дедупликация и сортировка имен для детерминированности
	uniqueMap := make(map[string]struct{}, len(fileNames))
	for _, fn := range fileNames {
		if fn != "" {
			uniqueMap[fn] = struct{}{}
		}
	}

	sortedNames := make([]string, 0, len(uniqueMap))
	for fn := range uniqueMap {
		sortedNames = append(sortedNames, fn)
	}
	sort.Strings(sortedNames)

	if len(sortedNames) == 0 {
		return []MediaItem{}, []string{}, 0, time.Since(startTime), nil
	}

	cleanDestBase, err := filepath.Abs(destMediaDir)
	if err != nil {
		return nil, nil, 0, 0, fmt.Errorf("не удалось определить абсолютный путь media: %w", err)
	}

	if err := os.MkdirAll(cleanDestBase, 0755); err != nil {
		return nil, nil, 0, 0, fmt.Errorf("не удалось создать каталог media: %w", err)
	}

	items := make([]MediaItem, 0, len(sortedNames))
	missing := make([]string, 0)
	var totalBytes int64

	for _, name := range sortedNames {
		if err := ValidateMediaFilename(name); err != nil {
			return nil, nil, 0, 0, err
		}

		srcPath := filepath.Join(mediaDir, name)
		srcInfo, err := os.Stat(srcPath)
		if err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, name)
				continue
			}
			return nil, nil, 0, 0, fmt.Errorf("ошибка доступа к исходному медиафайлу %s: %w", name, err)
		}

		if srcInfo.IsDir() {
			continue
		}

		dstPath := filepath.Join(cleanDestBase, name)
		cleanDstAbs, err := filepath.Abs(dstPath)
		if err != nil || (!strings.HasPrefix(cleanDstAbs, cleanDestBase+string(filepath.Separator)) && cleanDstAbs != cleanDestBase) {
			return nil, nil, 0, 0, fmt.Errorf("%w: целевой путь выходит за пределы media (%s)", ErrPathTraversal, name)
		}

		// Обеспечиваем создание поддиректорий, если имя содержит допустимую поддиректорию
		if err := os.MkdirAll(filepath.Dir(cleanDstAbs), 0755); err != nil {
			return nil, nil, 0, 0, fmt.Errorf("не удалось создать подкаталог для медиафайла %s: %w", name, err)
		}

		item, err := copyAndHashFile(srcPath, cleanDstAbs)
		if err != nil {
			return nil, nil, 0, 0, fmt.Errorf("ошибка копирования медиафайла %s: %w", name, err)
		}
		item.Name = name
		items = append(items, item)
		totalBytes += item.Size
	}

	// Детерминированная сортировка результатов
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	sort.Strings(missing)

	return items, missing, totalBytes, time.Since(startTime), nil
}

func copyAndHashFile(src, dst string) (MediaItem, error) {
	in, err := os.Open(src)
	if err != nil {
		return MediaItem{}, err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return MediaItem{}, err
	}
	defer out.Close()

	hasher := sha256.New()
	multiWriter := io.MultiWriter(out, hasher)

	written, err := io.Copy(multiWriter, in)
	if err != nil {
		return MediaItem{}, err
	}

	shaHex := hex.EncodeToString(hasher.Sum(nil))
	return MediaItem{
		Size:   written,
		SHA256: shaHex,
	}, nil
}
