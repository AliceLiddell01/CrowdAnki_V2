package export

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ExecuteExport координирует полный процесс экспорта CrowdAnki V2.
func ExecuteExport(req ExportRequest) (*ExportResult, error) {
	totalStart := time.Now()

	if req.DestinationDir == "" {
		return nil, fmt.Errorf("не указан целевой каталог экспорта (destination_dir)")
	}

	// 1. Проверка безопасности каталога назначения
	if err := EnsureSafeDestination(req.DestinationDir); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(req.DestinationDir, 0755); err != nil {
		return nil, fmt.Errorf("не удалось создать целевой каталог: %w", err)
	}

	// 2. Запись маркерного файла crowdanki.json
	if err := WriteManifestMeta(req.DestinationDir, req.RootDeckIDs); err != nil {
		return nil, fmt.Errorf("ошибка записи crowdanki.json: %w", err)
	}

	// 3. Запись decks.json
	if err := WriteDecks(req.DestinationDir, req.Decks); err != nil {
		return nil, fmt.Errorf("ошибка записи decks.json: %w", err)
	}

	// 4. Запись notes.jsonl
	if err := WriteNotesJSONL(req.DestinationDir, req.Notes); err != nil {
		return nil, fmt.Errorf("ошибка записи notes.jsonl: %w", err)
	}

	// 5. Запись cards.jsonl
	if err := WriteCardsJSONL(req.DestinationDir, req.Cards); err != nil {
		return nil, fmt.Errorf("ошибка записи cards.jsonl: %w", err)
	}

	// 6. Запись note_types/*.json
	if err := WriteNoteTypes(req.DestinationDir, req.NoteTypes); err != nil {
		return nil, fmt.Errorf("ошибка записи note_types: %w", err)
	}

	// 7. Обработка медиафайлов
	var mediaItems []MediaItem
	var missingMedia []string
	var mediaBytes int64
	var mediaDuration time.Duration

	if req.IncludeMedia && req.MediaDir != "" && len(req.MediaFiles) > 0 {
		destMediaDir := filepath.Join(req.DestinationDir, "media")
		var err error
		mediaItems, missingMedia, mediaBytes, mediaDuration, err = CopyMediaFiles(req.MediaDir, destMediaDir, req.MediaFiles)
		if err != nil {
			return nil, fmt.Errorf("ошибка при обработке медиафайлов: %w", err)
		}

		// Удаление устаревших управляемых медиафайлов от предыдущих экспортов
		if err := CleanStaleManagedMedia(req.DestinationDir, mediaItems); err != nil {
			return nil, fmt.Errorf("ошибка очистки устаревших медиафайлов: %w", err)
		}
	} else if req.IncludeMedia {
		// Очистка каталога media, если он существовал ранее, но сейчас файлов нет
		destMediaDir := filepath.Join(req.DestinationDir, "media")
		_ = os.RemoveAll(destMediaDir)
	}

	// 8. Запись media.json
	manifest := MediaManifest{
		Included: req.IncludeMedia,
		Files:    mediaItems,
		Missing:  missingMedia,
	}
	if manifest.Files == nil {
		manifest.Files = []MediaItem{}
	}
	if manifest.Missing == nil {
		manifest.Missing = []string{}
	}
	if err := WriteMediaManifest(req.DestinationDir, manifest); err != nil {
		return nil, fmt.Errorf("ошибка записи media.json: %w", err)
	}

	totalDuration := time.Since(totalStart)

	return &ExportResult{
		Decks:          len(req.Decks),
		Notes:          len(req.Notes),
		Cards:          len(req.Cards),
		NoteTypes:      len(req.NoteTypes),
		MediaFiles:     len(mediaItems),
		MediaBytes:     mediaBytes,
		MissingMedia:   len(missingMedia),
		ElapsedMs:      totalDuration.Milliseconds(),
		MediaElapsedMs: mediaDuration.Milliseconds(),
	}, nil
}
