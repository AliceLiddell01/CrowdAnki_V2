package export_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AliceLiddell01/CrowdAnki_V2/core/pkg/export"
)

// helperCreateSampleRequest создает базовую фикстуру для тестов
func helperCreateSampleRequest(destDir, mediaDir string, includeMedia bool) export.ExportRequest {
	parentID := "100"
	return export.ExportRequest{
		DestinationDir: destDir,
		MediaDir:       mediaDir,
		IncludeMedia:   includeMedia,
		RootDeckIDs:    []string{"100"},
		Decks: []export.DeckDTO{
			{ID: "200", ParentID: &parentID, Name: "Японский::Грамматика", Description: "Дочерняя колода"},
			{ID: "100", ParentID: nil, Name: "Японский", Description: "Корневая колода"},
			{ID: "300", ParentID: &parentID, Name: "Японский::Словарь", Description: "Вторая дочерняя колода"},
		},
		NoteTypes: []export.NoteTypeDTO{
			{
				ID:   "model_1",
				Name: "Основная (с медиа)",
				Fields: []export.NoteFieldDTO{
					{Name: "Лицо", Ord: 0},
					{Name: "Значение", Ord: 1},
				},
				Templates: []export.CardTemplateDTO{
					{Name: "Карточка 1", Ord: 0, Qfmt: "{{Лицо}}", Afmt: "{{FrontSide}}<hr id=answer>{{Значение}}"},
				},
				CSS: ".card { font-family: Meiryo; }",
			},
		},
		Notes: []export.NoteDTO{
			{
				GUID:       "note_guid_2",
				NoteTypeID: "model_1",
				NoteType:   "Основная (с медиа)",
				Fields: map[string]string{
					"Лицо":     "<b>猫</b> (ねこ)",
					"Значение": "Кот [sound:cat.mp3] <img src=\"cat.png\">",
				},
				Tags: []string{"n5", "animals"},
			},
			{
				GUID:       "note_guid_1",
				NoteTypeID: "model_1",
				NoteType:   "Основная (с медиа)",
				Fields: map[string]string{
					"Лицо":     "犬 (いぬ)",
					"Значение": "Собака [sound:dog.mp3]",
				},
				Tags: []string{"n5"},
			},
		},
		Cards: []export.CardDTO{
			{NoteGUID: "note_guid_1", CardID: "c1", Ord: 0, DeckID: "200"},
			{NoteGUID: "note_guid_2", CardID: "c2", Ord: 0, DeckID: "300"},
		},
		MediaFiles: []string{"cat.png", "cat.mp3", "dog.mp3", "missing.wav"},
	}
}

func TestExport_FileStructureAndContent(t *testing.T) {
	tempDir := t.TempDir()
	mediaSrcDir := filepath.Join(tempDir, "source_media")
	destDir := filepath.Join(tempDir, "export_output")

	if err := os.MkdirAll(mediaSrcDir, 0755); err != nil {
		t.Fatalf("не удалось создать папку source_media: %v", err)
	}

	// Создаем тестовые медиафайлы (cat.png, cat.mp3, dog.mp3). missing.wav намеренно отсутствует.
	_ = os.WriteFile(filepath.Join(mediaSrcDir, "cat.png"), []byte("PNG_CONTENT"), 0644)
	_ = os.WriteFile(filepath.Join(mediaSrcDir, "cat.mp3"), []byte("MP3_AUDIO"), 0644)
	_ = os.WriteFile(filepath.Join(mediaSrcDir, "dog.mp3"), []byte("DOG_AUDIO"), 0644)

	req := helperCreateSampleRequest(destDir, mediaSrcDir, true)
	// Добавим дубликат имени медиа для проверки дедупликации
	req.MediaFiles = append(req.MediaFiles, "cat.png", "cat.png")

	res, err := export.ExecuteExport(req)
	if err != nil {
		t.Fatalf("ExecuteExport завершился с ошибкой: %v", err)
	}

	// 1. Проверка статистических результатов
	if res.Decks != 3 {
		t.Errorf("ожидалось 3 колоды, получено: %d", res.Decks)
	}
	if res.Notes != 2 {
		t.Errorf("ожидалось 2 заметки, получено: %d", res.Notes)
	}
	if res.Cards != 2 {
		t.Errorf("ожидалось 2 карточки, получено: %d", res.Cards)
	}
	if res.NoteTypes != 1 {
		t.Errorf("ожидался 1 тип заметок, получено: %d", res.NoteTypes)
	}
	if res.MediaFiles != 3 {
		t.Errorf("ожидалось 3 медиафайла (дедуплицировано), получено: %d", res.MediaFiles)
	}
	if res.MissingMedia != 1 {
		t.Errorf("ожидался 1 отсутствующий медиафайл (missing.wav), получено: %d", res.MissingMedia)
	}

	// 2. Проверка наличия обязательных файлов
	expectedFiles := []string{
		"crowdanki.json",
		"decks.json",
		"notes.jsonl",
		"cards.jsonl",
		"media.json",
		filepath.Join("note_types", "model_1.json"),
		filepath.Join("media", "cat.png"),
		filepath.Join("media", "cat.mp3"),
		filepath.Join("media", "dog.mp3"),
	}
	for _, rel := range expectedFiles {
		fullPath := filepath.Join(destDir, rel)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("обязательный файл не создан: %s", rel)
		}
	}

	// Отсутствующий файл не должен быть создан
	if _, err := os.Stat(filepath.Join(destDir, "media", "missing.wav")); !os.IsNotExist(err) {
		t.Errorf("отсутствующий файл missing.wav был ошибочно создан в media/")
	}

	// 3. Проверка notes.jsonl: формат, Unicode, HTML, именованные поля
	notesData, err := os.ReadFile(filepath.Join(destDir, "notes.jsonl"))
	if err != nil {
		t.Fatalf("ошибка чтения notes.jsonl: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(notesData), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("в notes.jsonl ожидалось 2 строки, получено: %d", len(lines))
	}

	for _, line := range lines {
		var note export.NoteDTO
		if err := json.Unmarshal(line, &note); err != nil {
			t.Errorf("строка notes.jsonl не является валидным JSON: %v, строка: %s", err, string(line))
		}
		if note.GUID == "note_guid_2" {
			val, ok := note.Fields["Лицо"]
			if !ok || val != "<b>猫</b> (ねこ)" {
				t.Errorf("HTML/Unicode искажен в поле Лицо: %v", val)
			}
		}
	}

	// 4. Проверка decks.json: плоская структура, parent_id
	decksData, err := os.ReadFile(filepath.Join(destDir, "decks.json"))
	if err != nil {
		t.Fatalf("ошибка чтения decks.json: %v", err)
	}
	var decks []export.DeckDTO
	if err := json.Unmarshal(decksData, &decks); err != nil {
		t.Fatalf("ошибка парсинга decks.json: %v", err)
	}
	if len(decks) != 3 {
		t.Errorf("ожидалось 3 колоды в decks.json, получено: %d", len(decks))
	}

	// 5. Проверка media.json: manifest
	manifestData, err := os.ReadFile(filepath.Join(destDir, "media.json"))
	if err != nil {
		t.Fatalf("ошибка чтения media.json: %v", err)
	}
	var manifest export.MediaManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("ошибка парсинга media.json: %v", err)
	}
	if !manifest.Included {
		t.Errorf("ожидалось included=true в media.json")
	}
	if len(manifest.Files) != 3 {
		t.Errorf("ожидалось 3 файла в manifest, получено: %d", len(manifest.Files))
	}
	if len(manifest.Missing) != 1 || manifest.Missing[0] != "missing.wav" {
		t.Errorf("ожидался missing.wav в missing manifest, получено: %v", manifest.Missing)
	}
}

func TestExport_Determinism(t *testing.T) {
	tempDir := t.TempDir()
	mediaSrcDir := filepath.Join(tempDir, "source_media")
	destDir1 := filepath.Join(tempDir, "export1")
	destDir2 := filepath.Join(tempDir, "export2")

	_ = os.MkdirAll(mediaSrcDir, 0755)
	_ = os.WriteFile(filepath.Join(mediaSrcDir, "cat.png"), []byte("PNG_CONTENT"), 0644)

	req1 := helperCreateSampleRequest(destDir1, mediaSrcDir, true)
	req2 := helperCreateSampleRequest(destDir2, mediaSrcDir, true)

	if _, err := export.ExecuteExport(req1); err != nil {
		t.Fatalf("Экспорт 1 завершился с ошибкой: %v", err)
	}
	if _, err := export.ExecuteExport(req2); err != nil {
		t.Fatalf("Экспорт 2 завершился с ошибкой: %v", err)
	}

	compareFiles := []string{
		"crowdanki.json",
		"decks.json",
		"notes.jsonl",
		"cards.jsonl",
		"media.json",
		filepath.Join("note_types", "model_1.json"),
	}

	for _, rel := range compareFiles {
		f1, err := os.ReadFile(filepath.Join(destDir1, rel))
		if err != nil {
			t.Fatalf("ошибка чтения %s из export1: %v", rel, err)
		}
		f2, err := os.ReadFile(filepath.Join(destDir2, rel))
		if err != nil {
			t.Fatalf("ошибка чтения %s из export2: %v", rel, err)
		}
		if !bytes.Equal(f1, f2) {
			t.Errorf("файлы %s не идентичны побайтово при повторном экспорте!", rel)
		}
	}
}

func TestExport_MediaDisabled(t *testing.T) {
	tempDir := t.TempDir()
	destDir := filepath.Join(tempDir, "export_no_media")
	req := helperCreateSampleRequest(destDir, "", false)

	res, err := export.ExecuteExport(req)
	if err != nil {
		t.Fatalf("ExecuteExport с выключенным media завершился ошибкой: %v", err)
	}

	if res.MediaFiles != 0 {
		t.Errorf("ожидалось 0 media_files, получено: %d", res.MediaFiles)
	}

	manifestData, err := os.ReadFile(filepath.Join(destDir, "media.json"))
	if err != nil {
		t.Fatalf("ошибка чтения media.json: %v", err)
	}
	var manifest export.MediaManifest
	_ = json.Unmarshal(manifestData, &manifest)
	if manifest.Included {
		t.Errorf("ожидалось included=false в media.json при выключенном media")
	}
	if len(manifest.Files) != 0 {
		t.Errorf("ожидался пустой список files в media.json")
	}

	// Папка media не должна быть создана
	if _, err := os.Stat(filepath.Join(destDir, "media")); !os.IsNotExist(err) {
		t.Errorf("каталог media/ не должен существовать при выключенном медиа")
	}
}

func TestExport_UnsafeDirectoryProtection(t *testing.T) {
	tempDir := t.TempDir()
	foreignFile := filepath.Join(tempDir, "user_important_file.txt")
	_ = os.WriteFile(foreignFile, []byte("IMPORTANT USER DATA"), 0644)

	req := helperCreateSampleRequest(tempDir, "", false)
	_, err := export.ExecuteExport(req)
	if err == nil {
		t.Fatalf("ожидалась ошибка защиты от записи в чужой непустой каталог, но экспорт прошел успешно")
	}

	// Проверяем, что исходный файл пользователя не пострадал
	content, readErr := os.ReadFile(foreignFile)
	if readErr != nil || string(content) != "IMPORTANT USER DATA" {
		t.Errorf("чужой пользовательский файл был поврежден или удален!")
	}
}

func TestExport_PathTraversalProtection(t *testing.T) {
	tempDir := t.TempDir()
	destDir := filepath.Join(tempDir, "export")
	mediaDir := filepath.Join(tempDir, "media")
	_ = os.MkdirAll(mediaDir, 0755)

	req := helperCreateSampleRequest(destDir, mediaDir, true)
	req.MediaFiles = []string{"../dangerous.txt", "/etc/passwd", "..\\system.ini"}

	_, err := export.ExecuteExport(req)
	if err == nil {
		t.Fatalf("ожидалась ошибка path traversal, но экспорт прошел успешно")
	}
}
