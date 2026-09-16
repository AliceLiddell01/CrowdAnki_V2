package export

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
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

	numWorkers := runtime.NumCPU() * 2
	if numWorkers < 4 {
		numWorkers = 4
	}
	if numWorkers > 32 {
		numWorkers = 32
	}
	if numWorkers > len(sortedNames) {
		numWorkers = len(sortedNames)
	}

	type fileResult struct {
		item    *MediaItem
		missing string
		err     error
	}

	jobs := make(chan string, len(sortedNames))
	results := make(chan fileResult, len(sortedNames))

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range jobs {
				if err := ValidateMediaFilename(name); err != nil {
					results <- fileResult{err: err}
					return
				}

				srcPath := filepath.Join(mediaDir, name)
				srcInfo, err := os.Stat(srcPath)
				if err != nil {
					if os.IsNotExist(err) {
						results <- fileResult{missing: name}
						continue
					}
					results <- fileResult{err: fmt.Errorf("ошибка доступа к исходному медиафайлу %s: %w", name, err)}
					return
				}

				if srcInfo.IsDir() {
					continue
				}

				dstPath := filepath.Join(cleanDestBase, name)
				cleanDstAbs, err := filepath.Abs(dstPath)
				if err != nil || (!strings.HasPrefix(cleanDstAbs, cleanDestBase+string(filepath.Separator)) && cleanDstAbs != cleanDestBase) {
					results <- fileResult{err: fmt.Errorf("%w: целевой путь выходит за пределы media (%s)", ErrPathTraversal, name)}
					return
				}

				dir := filepath.Dir(cleanDstAbs)
				if dir != cleanDestBase {
					if err := os.MkdirAll(dir, 0755); err != nil {
						results <- fileResult{err: fmt.Errorf("не удалось создать подкаталог для медиафайла %s: %w", name, err)}
						return
					}
				}

				item, err := copyAndHashFile(srcPath, cleanDstAbs)
				if err != nil {
					results <- fileResult{err: fmt.Errorf("ошибка копирования медиафайла %s: %w", name, err)}
					return
				}
				item.Name = name
				results <- fileResult{item: &item}
			}
		}()
	}

	for _, name := range sortedNames {
		jobs <- name
	}
	close(jobs)

	wg.Wait()
	close(results)

	items := make([]MediaItem, 0, len(sortedNames))
	missing := make([]string, 0)
	var totalBytes int64

	for res := range results {
		if res.err != nil {
			return nil, nil, 0, 0, res.err
		}
		if res.missing != "" {
			missing = append(missing, res.missing)
		}
		if res.item != nil {
			items = append(items, *res.item)
			totalBytes += res.item.Size
		}
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
