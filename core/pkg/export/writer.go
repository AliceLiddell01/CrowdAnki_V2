package export

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var (
	// ErrUnsafeDirectory сигнализирует о том, что выбранный каталог не пуст и не является проектом CrowdAnki V2.
	ErrUnsafeDirectory = errors.New("каталог не пуст и не содержит валидный проект CrowdAnki V2 (crowdanki.json)")
	// ErrUnsupportedSchemaVersion сигнализирует о несовместимой версии схемы существующего экспорта.
	ErrUnsupportedSchemaVersion = errors.New("неподдерживаемая версия схемы экспорта CrowdAnki V2")
)

// SupportedSchemaVersion определяет текущую поддерживаемую версию схемы.
const SupportedSchemaVersion = 1

// ManagedRootEntries определяет список файлов и каталогов, управляемых CrowdAnki V2 в корне экспорта.
var ManagedRootEntries = map[string]bool{
	"crowdanki.json": true,
	"decks.json":     true,
	"notes.jsonl":    true,
	"cards.jsonl":    true,
	"media.json":     true,
	"note_types":     true,
	"media":          true,
}

// EnsureSafeDestination проверяет безопасность каталога назначения.
// Разрешены: пустой каталог или каталог, уже содержащий валидный crowdanki.json с поддерживаемой версией схемы.
func EnsureSafeDestination(destDir string) error {
	entries, err := os.ReadDir(destDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("не удалось прочитать каталог назначения: %w", err)
	}

	if len(entries) == 0 {
		return nil
	}

	// Если каталог не пуст, в нем обязан быть валидный crowdanki.json
	markerPath := filepath.Join(destDir, "crowdanki.json")
	data, err := os.ReadFile(markerPath)
	if err != nil {
		return fmt.Errorf("%w: отсутствует crowdanki.json в каталоге %s", ErrUnsafeDirectory, destDir)
	}

	var meta ManifestMeta
	if err := json.Unmarshal(data, &meta); err != nil || meta.Format != "crowdanki-v2" {
		return fmt.Errorf("%w: некорректный маркер crowdanki.json в каталоге %s", ErrUnsafeDirectory, destDir)
	}

	if meta.SchemaVersion != SupportedSchemaVersion {
		return fmt.Errorf("%w: версия схемы %d не поддерживается (ожидается %d) в %s",
			ErrUnsupportedSchemaVersion, meta.SchemaVersion, SupportedSchemaVersion, destDir)
	}

	return nil
}

// WriteManifestMeta записывает crowdanki.json.
func WriteManifestMeta(destDir string, rootDecks []string) error {
	sortedRoots := make([]string, len(rootDecks))
	copy(sortedRoots, rootDecks)
	sort.Strings(sortedRoots)

	meta := ManifestMeta{
		Format:        "crowdanki-v2",
		SchemaVersion: SupportedSchemaVersion,
		RootDecks:     sortedRoots,
	}

	return writeJSONFile(filepath.Join(destDir, "crowdanki.json"), meta)
}

// WriteDecks записывает decks.json в детерминированном плоском виде.
func WriteDecks(destDir string, decks []DeckDTO) error {
	sortedDecks := make([]DeckDTO, len(decks))
	copy(sortedDecks, decks)

	// Сортировка по имени, затем по ID для гарантированной детерминированности
	sort.Slice(sortedDecks, func(i, j int) bool {
		if sortedDecks[i].Name != sortedDecks[j].Name {
			return sortedDecks[i].Name < sortedDecks[j].Name
		}
		return sortedDecks[i].ID < sortedDecks[j].ID
	})

	return writeJSONFile(filepath.Join(destDir, "decks.json"), sortedDecks)
}

// WriteNotesJSONL записывает notes.jsonl. Каждая строка — отдельный валидный JSON-объект.
func WriteNotesJSONL(destDir string, notes []NoteDTO) error {
	sortedNotes := make([]NoteDTO, len(notes))
	copy(sortedNotes, notes)

	sort.Slice(sortedNotes, func(i, j int) bool {
		return sortedNotes[i].GUID < sortedNotes[j].GUID
	})

	filePath := filepath.Join(destDir, "notes.jsonl")
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("не удалось создать notes.jsonl: %w", err)
	}
	defer file.Close()

	for _, note := range sortedNotes {
		// Детерминированность тегов
		sort.Strings(note.Tags)

		lineBytes, err := json.Marshal(note)
		if err != nil {
			return fmt.Errorf("ошибка сериализации заметки %s: %w", note.GUID, err)
		}

		if _, err := file.Write(lineBytes); err != nil {
			return fmt.Errorf("ошибка записи notes.jsonl: %w", err)
		}
		if _, err := file.WriteString("\n"); err != nil {
			return fmt.Errorf("ошибка записи перевода строки в notes.jsonl: %w", err)
		}
	}

	return file.Sync()
}

// WriteCardsJSONL записывает cards.jsonl.
func WriteCardsJSONL(destDir string, cards []CardDTO) error {
	sortedCards := make([]CardDTO, len(cards))
	copy(sortedCards, cards)

	sort.Slice(sortedCards, func(i, j int) bool {
		if sortedCards[i].NoteGUID != sortedCards[j].NoteGUID {
			return sortedCards[i].NoteGUID < sortedCards[j].NoteGUID
		}
		if sortedCards[i].Ord != sortedCards[j].Ord {
			return sortedCards[i].Ord < sortedCards[j].Ord
		}
		return sortedCards[i].CardID < sortedCards[j].CardID
	})

	filePath := filepath.Join(destDir, "cards.jsonl")
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("не удалось создать cards.jsonl: %w", err)
	}
	defer file.Close()

	for _, card := range sortedCards {
		lineBytes, err := json.Marshal(card)
		if err != nil {
			return fmt.Errorf("ошибка сериализации карточки: %w", err)
		}

		if _, err := file.Write(lineBytes); err != nil {
			return fmt.Errorf("ошибка записи cards.jsonl: %w", err)
		}
		if _, err := file.WriteString("\n"); err != nil {
			return fmt.Errorf("ошибка записи перевода строки в cards.jsonl: %w", err)
		}
	}

	return file.Sync()
}

// WriteNoteTypes записывает каждый используемый тип заметки в отдельный JSON-файл в note_types/<id>.json.
// При повторном экспорте удаляются файлы типов заметок, которые более не используются.
func WriteNoteTypes(destDir string, noteTypes []NoteTypeDTO) error {
	noteTypesDir := filepath.Join(destDir, "note_types")
	if err := os.MkdirAll(noteTypesDir, 0755); err != nil {
		return fmt.Errorf("не удалось создать каталог note_types: %w", err)
	}

	expectedFiles := make(map[string]bool, len(noteTypes))

	for _, nt := range noteTypes {
		fileName := fmt.Sprintf("%s.json", nt.ID)
		expectedFiles[fileName] = true

		filePath := filepath.Join(noteTypesDir, fileName)
		if err := writeJSONFile(filePath, nt); err != nil {
			return fmt.Errorf("ошибка записи типа заметки %s: %w", nt.ID, err)
		}
	}

	// Очистка устаревших типов заметок
	entries, err := os.ReadDir(noteTypesDir)
	if err != nil {
		return fmt.Errorf("не удалось прочитать каталог note_types для синхронизации: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			if !expectedFiles[entry.Name()] {
				if err := os.Remove(filepath.Join(noteTypesDir, entry.Name())); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("ошибка удаления устаревшего типа заметки %s: %w", entry.Name(), err)
				}
			}
		}
	}

	return nil
}

// WriteMediaManifest записывает media.json.
func WriteMediaManifest(destDir string, manifest MediaManifest) error {
	// Сортировка для детерминированности
	sort.Slice(manifest.Files, func(i, j int) bool {
		return manifest.Files[i].Name < manifest.Files[j].Name
	})
	sort.Strings(manifest.Missing)

	return writeJSONFile(filepath.Join(destDir, "media.json"), manifest)
}

// CleanStaleManagedMedia удаляет из destDir/media файлы, которых больше нет в актуальном манифесте.
func CleanStaleManagedMedia(destDir string, validFiles []MediaItem) error {
	mediaDir := filepath.Join(destDir, "media")
	if _, err := os.Stat(mediaDir); os.IsNotExist(err) {
		return nil
	}

	validSet := make(map[string]bool, len(validFiles))
	for _, vf := range validFiles {
		validSet[filepath.Clean(vf.Name)] = true
	}

	return filepath.Walk(mediaDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(mediaDir, path)
		if err != nil {
			return err
		}
		cleanRel := filepath.Clean(rel)
		if !validSet[cleanRel] {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("ошибка удаления устаревшего медиафайла %s: %w", cleanRel, err)
			}
		}
		return nil
	})
}

// CommitStagedExport атомарно переносит файлы из stagingDir в destDir,
// удаляя или синхронизируя только управляемые сущности CrowdAnki V2 и сохраняя посторонние файлы пользователя.
func CommitStagedExport(stagingDir, destDir string, includeMedia bool) error {
	managedFiles := []string{
		"crowdanki.json",
		"decks.json",
		"notes.jsonl",
		"cards.jsonl",
		"media.json",
	}

	// 1. Копируем основные файлы
	for _, fname := range managedFiles {
		src := filepath.Join(stagingDir, fname)
		dst := filepath.Join(destDir, fname)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("ошибка переноса файла %s в целевой каталог: %w", fname, err)
		}
	}

	// 2. Синхронизируем note_types/
	dstNoteTypes := filepath.Join(destDir, "note_types")
	if err := os.MkdirAll(dstNoteTypes, 0755); err != nil {
		return fmt.Errorf("не удалось создать целевой каталог note_types: %w", err)
	}
	srcNoteTypes := filepath.Join(stagingDir, "note_types")
	stagedEntries, err := os.ReadDir(srcNoteTypes)
	if err == nil {
		stagedMap := make(map[string]bool, len(stagedEntries))
		for _, e := range stagedEntries {
			stagedMap[e.Name()] = true
			if err := copyFile(filepath.Join(srcNoteTypes, e.Name()), filepath.Join(dstNoteTypes, e.Name())); err != nil {
				return fmt.Errorf("ошибка переноса типа заметки %s: %w", e.Name(), err)
			}
		}
		// Удаляем устаревшие типы заметок в dst
		dstEntries, err := os.ReadDir(dstNoteTypes)
		if err == nil {
			for _, de := range dstEntries {
				if !stagedMap[de.Name()] && filepath.Ext(de.Name()) == ".json" {
					if err := os.Remove(filepath.Join(dstNoteTypes, de.Name())); err != nil && !os.IsNotExist(err) {
						return fmt.Errorf("ошибка удаления устаревшего типа заметки %s: %w", de.Name(), err)
					}
				}
			}
		}
	}

	// 3. Синхронизируем media/
	dstMedia := filepath.Join(destDir, "media")
	if !includeMedia {
		// При выключенных медиа управляемый каталог media должен быть полностью очищен
		if err := os.RemoveAll(dstMedia); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("ошибка очистки устаревшего каталога media: %w", err)
		}
	} else {
		srcMedia := filepath.Join(stagingDir, "media")
		if _, err := os.Stat(srcMedia); err == nil {
			if err := os.MkdirAll(dstMedia, 0755); err != nil {
				return fmt.Errorf("не удалось создать целевой каталог media: %w", err)
			}
			stagedMediaFiles := make(map[string]bool)
			err := filepath.Walk(srcMedia, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return err
				}
				rel, err := filepath.Rel(srcMedia, path)
				if err != nil {
					return err
				}
				stagedMediaFiles[filepath.Clean(rel)] = true
				targetPath := filepath.Join(dstMedia, rel)
				if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
					return err
				}
				return copyFile(path, targetPath)
			})
			if err != nil {
				return fmt.Errorf("ошибка переноса медиафайлов: %w", err)
			}
			// Удаляем устаревшие файлы в dstMedia
			_ = filepath.Walk(dstMedia, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return err
				}
				rel, err := filepath.Rel(dstMedia, path)
				if err != nil {
					return err
				}
				if !stagedMediaFiles[filepath.Clean(rel)] {
					if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
						return fmt.Errorf("ошибка удаления устаревшего медиафайла %s: %w", rel, err)
					}
				}
				return nil
			})
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func writeJSONFile(filePath string, data any) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false) // Сохраняем HTML без экранирования & < >

	if err := encoder.Encode(data); err != nil {
		return err
	}

	return file.Sync()
}
