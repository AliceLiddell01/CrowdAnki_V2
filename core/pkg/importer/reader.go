package importer

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AliceLiddell01/CrowdAnki_V2/core/pkg/export"
)

var (
	// ErrInvalidFormat указывает на неверный формат проекта.
	ErrInvalidFormat = errors.New("каталог не содержит валидный проект CrowdAnki V2")
	// ErrUnsupportedSchema указывает на несовместимую версию схемы проекта.
	ErrUnsupportedSchema = errors.New("неподдерживаемая версия схемы проекта CrowdAnki V2")
	// ErrValidationError указывает на ошибку целостности или валидации данных проекта.
	ErrValidationError = errors.New("ошибка валидации структуры проекта")
)

// SourceProject содержит распарсенные и валидированные данные импортируемого проекта.
type SourceProject struct {
	Manifest   export.ManifestMeta
	Decks      []export.DeckDTO
	DeckByID   map[string]export.DeckDTO
	DeckByName map[string]export.DeckDTO
	NoteTypes  map[string]export.NoteTypeDTO
	Notes      []export.NoteDTO
	NoteByGUID map[string]export.NoteDTO
	Cards      []export.CardDTO
	Media      *export.MediaManifest
	SourceDir  string
	MediaDir   string
}

// ReadAndValidateSourceProject читает и строго валидирует каталог проекта CrowdAnki V2.
func ReadAndValidateSourceProject(sourceDir string) (*SourceProject, []string, error) {
	var validationErrors []string

	// 1. Проверка crowdanki.json
	markerPath := filepath.Join(sourceDir, "crowdanki.json")
	markerData, err := os.ReadFile(markerPath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: отсутствует crowdanki.json в каталоге %s", ErrInvalidFormat, sourceDir)
	}

	var manifest export.ManifestMeta
	if err := json.Unmarshal(markerData, &manifest); err != nil {
		return nil, nil, fmt.Errorf("%w: некорректный JSON в crowdanki.json: %v", ErrInvalidFormat, err)
	}

	if manifest.Format != "crowdanki-v2" {
		return nil, nil, fmt.Errorf("%w: ожидался формат 'crowdanki-v2', получено '%s'", ErrInvalidFormat, manifest.Format)
	}

	if manifest.SchemaVersion != export.SupportedSchemaVersion {
		return nil, nil, fmt.Errorf("%w: версия схемы %d не поддерживается (поддерживается %d)", ErrUnsupportedSchema, manifest.SchemaVersion, export.SupportedSchemaVersion)
	}

	if len(manifest.RootDecks) == 0 {
		return nil, nil, fmt.Errorf("%w: список root_decks в crowdanki.json пуст", ErrValidationError)
	}

	// 2. Чтение decks.json
	decksPath := filepath.Join(sourceDir, "decks.json")
	decksData, err := os.ReadFile(decksPath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: отсутствует decks.json в каталоге %s", ErrValidationError, sourceDir)
	}

	var decks []export.DeckDTO
	if err := json.Unmarshal(decksData, &decks); err != nil {
		return nil, nil, fmt.Errorf("%w: ошибка разбора decks.json: %v", ErrValidationError, err)
	}

	deckByID := make(map[string]export.DeckDTO, len(decks))
	deckByName := make(map[string]export.DeckDTO, len(decks))
	for _, d := range decks {
		if _, exists := deckByID[d.ID]; exists {
			validationErrors = append(validationErrors, fmt.Sprintf("Дублирующийся ID колоды в decks.json: %s", d.ID))
		}
		deckByID[d.ID] = d
		deckByName[d.Name] = d
	}

	// Проверка отсутствия циклов в иерархии колод
	for _, d := range decks {
		visited := make(map[string]bool)
		curr := d
		for curr.ParentID != nil {
			if visited[curr.ID] {
				validationErrors = append(validationErrors, fmt.Sprintf("Обнаружен цикл в иерархии колод для колоды '%s' (ID: %s)", d.Name, d.ID))
				break
			}
			visited[curr.ID] = true
			parent, ok := deckByID[*curr.ParentID]
			if !ok {
				validationErrors = append(validationErrors, fmt.Sprintf("Колода '%s' ссылается на несуществующего родителя ID %s", curr.Name, *curr.ParentID))
				break
			}
			curr = parent
		}
	}

	// Проверка наличия корневых колод из manifest
	for _, rootName := range manifest.RootDecks {
		if _, ok := deckByName[rootName]; !ok {
			validationErrors = append(validationErrors, fmt.Sprintf("Корневая колода '%s' из crowdanki.json отсутствует в decks.json", rootName))
		}
	}

	// 3. Чтение note_types/
	noteTypesDir := filepath.Join(sourceDir, "note_types")
	ntEntries, err := os.ReadDir(noteTypesDir)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: отсутствует каталог note_types в каталоге %s", ErrValidationError, sourceDir)
	}

	noteTypes := make(map[string]export.NoteTypeDTO)
	for _, entry := range ntEntries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		ntData, err := os.ReadFile(filepath.Join(noteTypesDir, entry.Name()))
		if err != nil {
			validationErrors = append(validationErrors, fmt.Sprintf("Не удалось прочитать тип заметки %s: %v", entry.Name(), err))
			continue
		}
		var nt export.NoteTypeDTO
		if err := json.Unmarshal(ntData, &nt); err != nil {
			validationErrors = append(validationErrors, fmt.Sprintf("Некорректный JSON в файле типа заметки %s: %v", entry.Name(), err))
			continue
		}
		noteTypes[nt.ID] = nt
	}

	// 4. Чтение notes.jsonl
	notesPath := filepath.Join(sourceDir, "notes.jsonl")
	notesFile, err := os.Open(notesPath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: отсутствует notes.jsonl в каталоге %s", ErrValidationError, sourceDir)
	}
	defer notesFile.Close()

	var notes []export.NoteDTO
	noteByGUID := make(map[string]export.NoteDTO)
	scanner := bufio.NewScanner(notesFile)
	// Увеличиваем буфер для потенциально крупных заметок
	const maxScanCapacity = 10 * 1024 * 1024
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxScanCapacity)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var note export.NoteDTO
		if err := json.Unmarshal([]byte(line), &note); err != nil {
			validationErrors = append(validationErrors, fmt.Sprintf("Ошибка разбора JSON в notes.jsonl (строка %d): %v", lineNum, err))
			continue
		}
		if _, exists := noteByGUID[note.GUID]; exists {
			validationErrors = append(validationErrors, fmt.Sprintf("Дублирующийся GUID заметки в notes.jsonl (строка %d): %s", lineNum, note.GUID))
		}
		if _, ok := noteTypes[note.NoteTypeID]; !ok {
			validationErrors = append(validationErrors, fmt.Sprintf("Заметка %s ссылается на неизвестный тип заметки ID %s (строка %d)", note.GUID, note.NoteTypeID, lineNum))
		}
		noteByGUID[note.GUID] = note
		notes = append(notes, note)
	}
	if err := scanner.Err(); err != nil {
		validationErrors = append(validationErrors, fmt.Sprintf("Ошибка чтения notes.jsonl: %v", err))
	}

	// 5. Чтение cards.jsonl
	cardsPath := filepath.Join(sourceDir, "cards.jsonl")
	cardsFile, err := os.Open(cardsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: отсутствует cards.jsonl в каталоге %s", ErrValidationError, sourceDir)
	}
	defer cardsFile.Close()

	var cards []export.CardDTO
	cardScanner := bufio.NewScanner(cardsFile)
	cardScanner.Buffer(buf, maxScanCapacity)
	cardLineNum := 0
	seenLogicalCards := make(map[string]bool)

	for cardScanner.Scan() {
		cardLineNum++
		line := strings.TrimSpace(cardScanner.Text())
		if line == "" {
			continue
		}
		var card export.CardDTO
		if err := json.Unmarshal([]byte(line), &card); err != nil {
			validationErrors = append(validationErrors, fmt.Sprintf("Ошибка разбора JSON в cards.jsonl (строка %d): %v", cardLineNum, err))
			continue
		}
		logicalKey := fmt.Sprintf("%s:%d", card.NoteGUID, card.Ord)
		if seenLogicalCards[logicalKey] {
			validationErrors = append(validationErrors, fmt.Sprintf("Дублирующаяся логическая карточка (GUID: %s, Ord: %d) в cards.jsonl (строка %d)", card.NoteGUID, card.Ord, cardLineNum))
		}
		seenLogicalCards[logicalKey] = true

		if _, ok := noteByGUID[card.NoteGUID]; !ok {
			validationErrors = append(validationErrors, fmt.Sprintf("Карточка ссылается на несуществующую заметку GUID %s (строка %d)", card.NoteGUID, cardLineNum))
		}
		if _, ok := deckByID[card.DeckID]; !ok {
			validationErrors = append(validationErrors, fmt.Sprintf("Карточка ссылается на несуществующую колоду ID %s (строка %d)", card.DeckID, cardLineNum))
		}

		cards = append(cards, card)
	}
	if err := cardScanner.Err(); err != nil {
		validationErrors = append(validationErrors, fmt.Sprintf("Ошибка чтения cards.jsonl: %v", err))
	}

	// 6. Чтение media.json (опционально)
	var mediaManifest *export.MediaManifest
	mediaJSONPath := filepath.Join(sourceDir, "media.json")
	if mediaData, err := os.ReadFile(mediaJSONPath); err == nil {
		var mm export.MediaManifest
		if err := json.Unmarshal(mediaData, &mm); err == nil {
			mediaManifest = &mm
			// Проверка безопасности путей (защита от path traversal)
			projectMediaDir := filepath.Join(sourceDir, "media")
			for _, mf := range mm.Files {
				if err := validateMediaPath(projectMediaDir, mf.Name); err != nil {
					validationErrors = append(validationErrors, fmt.Sprintf("Небезопасный путь к медиафайлу '%s': %v", mf.Name, err))
				}
			}
		} else {
			validationErrors = append(validationErrors, fmt.Sprintf("Некорректный JSON в media.json: %v", err))
		}
	}

	if len(validationErrors) > 0 {
		return nil, validationErrors, fmt.Errorf("%w: обнаружены ошибки валидации структуры проекта (%d)", ErrValidationError, len(validationErrors))
	}

	return &SourceProject{
		Manifest:   manifest,
		Decks:      decks,
		DeckByID:   deckByID,
		DeckByName: deckByName,
		NoteTypes:  noteTypes,
		Notes:      notes,
		NoteByGUID: noteByGUID,
		Cards:      cards,
		Media:      mediaManifest,
		SourceDir:  sourceDir,
		MediaDir:   filepath.Join(sourceDir, "media"),
	}, nil, nil
}

// validateMediaPath проверяет, что имя файла не содержит path traversal и не выходит за пределы baseDir.
func validateMediaPath(baseDir, relName string) error {
	if strings.Contains(relName, "..") {
		return fmt.Errorf("путь содержит запрещенный сегмент '..'")
	}
	if filepath.IsAbs(relName) {
		return fmt.Errorf("путь не должен быть абсолютным")
	}
	cleaned := filepath.Clean(relName)
	if strings.HasPrefix(cleaned, "..") || strings.HasPrefix(cleaned, string(filepath.Separator)) {
		return fmt.Errorf("путь выходит за пределы каталога медиафайлов")
	}

	target := filepath.Join(baseDir, cleaned)
	rel, err := filepath.Rel(baseDir, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("путь выходит за пределы базового каталога")
	}

	return nil
}
