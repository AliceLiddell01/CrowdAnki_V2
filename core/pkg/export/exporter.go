package export

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ExecuteExport координирует полный процесс экспорта CrowdAnki V2 с использованием staging-каталога.
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

	// Создаем изолированный временный staging-каталог внутри целевой папки,
	// чтобы гарантировать один том и быстрый атомарный rename без повторного копирования файлов
	stagingDir, err := os.MkdirTemp(req.DestinationDir, ".crowdanki-staging-*")
	if err != nil {
		stagingDir, err = os.MkdirTemp("", "crowdanki-export-staging-*")
		if err != nil {
			return nil, fmt.Errorf("не удалось создать временный staging-каталог: %w", err)
		}
	}
	defer func() {
		_ = os.RemoveAll(stagingDir)
	}()

	// 2. Запись маркерного файла crowdanki.json в staging
	if err := WriteManifestMeta(stagingDir, req.RootDeckIDs); err != nil {
		return nil, fmt.Errorf("ошибка записи crowdanki.json: %w", err)
	}

	// 3. Запись decks.json в staging
	if err := WriteDecks(stagingDir, req.Decks); err != nil {
		return nil, fmt.Errorf("ошибка записи decks.json: %w", err)
	}

	// 4. Запись notes.jsonl в staging
	if err := WriteNotesJSONL(stagingDir, req.Notes); err != nil {
		return nil, fmt.Errorf("ошибка записи notes.jsonl: %w", err)
	}

	// 5. Запись cards.jsonl в staging
	if err := WriteCardsJSONL(stagingDir, req.Cards); err != nil {
		return nil, fmt.Errorf("ошибка записи cards.jsonl: %w", err)
	}

	// 6. Запись note_types/*.json в staging
	if err := WriteNoteTypes(stagingDir, req.NoteTypes); err != nil {
		return nil, fmt.Errorf("ошибка записи note_types: %w", err)
	}

	// 7. Обработка медиафайлов в staging
	var mediaItems []MediaItem
	var missingMedia []string
	var mediaBytes int64
	var mediaDuration time.Duration

	if req.IncludeMedia && req.MediaDir != "" && len(req.MediaFiles) > 0 {
		stagedMediaDir := filepath.Join(stagingDir, "media")
		var err error
		mediaItems, missingMedia, mediaBytes, mediaDuration, err = CopyMediaFiles(req.MediaDir, stagedMediaDir, req.MediaFiles)
		if err != nil {
			return nil, fmt.Errorf("ошибка при обработке медиафайлов: %w", err)
		}
	}

	// 8. Запись media.json в staging
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
	if err := WriteMediaManifest(stagingDir, manifest); err != nil {
		return nil, fmt.Errorf("ошибка записи media.json: %w", err)
	}

	// 9. Фиксация результата: перенос только управляемых сущностей из staging в реальный destination
	if err := CommitStagedExport(stagingDir, req.DestinationDir, req.IncludeMedia); err != nil {
		return nil, fmt.Errorf("ошибка фиксации экспорта: %w", err)
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
