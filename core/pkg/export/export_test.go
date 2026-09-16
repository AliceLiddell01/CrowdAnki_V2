package export_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AliceLiddell01/CrowdAnki_V2/core/pkg/export"
)

// helperCreateRichFixture создает исчерпывающую фикстуру согласно спецификации
func helperCreateRichFixture(destDir, mediaDir string, includeMedia bool) export.ExportRequest {
	rootID := "100"
	childID := "200"

	return export.ExportRequest{
		DestinationDir: destDir,
		MediaDir:       mediaDir,
		IncludeMedia:   includeMedia,
		RootDeckIDs:    []string{"100"},
		Decks: []export.DeckDTO{
			{ID: "200", ParentID: &rootID, Name: "Японский::N5", Description: "Дочерняя колода N5"},
			{ID: "300", ParentID: &childID, Name: "Японский::N5::Кандзи", Description: "Внучатая колода Кандзи"},
			{ID: "100", ParentID: nil, Name: "Японский", Description: "Корневая колода"},
		},
		NoteTypes: []export.NoteTypeDTO{
			{
				ID:    "model_std",
				Name:  "Основная (с медиа)",
				Kind:  "standard",
				Sortf: 0,
				Fields: []export.NoteFieldDTO{
					{Name: "Лицо", Ord: 0},
					{Name: "Значение", Ord: 1},
				},
				Templates: []export.CardTemplateDTO{
					{Name: "Прямая", Ord: 0, Qfmt: "{{Лицо}}", Afmt: "{{FrontSide}}<hr id=answer>{{Значение}}"},
					{Name: "Обратная", Ord: 1, Qfmt: "{{Значение}}", Afmt: "{{FrontSide}}<hr id=answer>{{Лицо}}"},
				},
				CSS: ".card { font-family: Meiryo; }",
			},
			{
				ID:    "model_cloze",
				Name:  "С пропусками (Cloze)",
				Kind:  "cloze",
				Sortf: 0,
				Fields: []export.NoteFieldDTO{
					{Name: "Текст", Ord: 0},
					{Name: "Комментарий", Ord: 1},
				},
				Templates: []export.CardTemplateDTO{
					{Name: "Клоуз", Ord: 0, Qfmt: "{{cloze:Текст}}", Afmt: "{{cloze:Текст}}<br>{{Комментарий}}"},
				},
				CSS: ".cloze { color: blue; }",
			},
		},
		Notes: []export.NoteDTO{
			{
				GUID:       "note_guid_multi_cards",
				NoteTypeID: "model_std",
				NoteType:   "Основная (с медиа)",
				Fields: map[string]string{
					"Лицо":     "<b>猫</b> (ねко)",
					"Значение": "Кот [sound:cat.mp3] <img src=\"cat.png\">",
				},
				Tags: []string{"n5", "animals"},
			},
			{
				GUID:       "note_guid_cloze",
				NoteTypeID: "model_cloze",
				NoteType:   "С пропусками (Cloze)",
				Fields: map[string]string{
					"Текст":       "{{c1::犬}} (собака) [sound:cat.mp3]", // Дубликат ссылки на cat.mp3
					"Комментарий": "Статический шрифт <img src=\"_font.png\">",
				},
				Tags: []string{"n5", "cloze_test"},
			},
		},
		Cards: []export.CardDTO{
			// Две карточки для одной заметки в разных колодах
			{NoteGUID: "note_guid_multi_cards", CardID: "c1", Ord: 0, DeckID: "200"},
			{NoteGUID: "note_guid_multi_cards", CardID: "c2", Ord: 1, DeckID: "300"},
			// Карточка для заметки cloze
			{NoteGUID: "note_guid_cloze", CardID: "c3", Ord: 0, DeckID: "200"},
		},
		MediaFiles: []string{"cat.png", "cat.mp3", "_font.png", "missing.wav"},
	}
}

func TestExport_RichFixtureAndInvariants(t *testing.T) {
	tempDir := t.TempDir()
	mediaSrcDir := filepath.Join(tempDir, "source_media")
	destDir := filepath.Join(tempDir, "export_output")

	if err := os.MkdirAll(mediaSrcDir, 0755); err != nil {
		t.Fatalf("не удалось создать папку source_media: %v", err)
	}

	catPngBytes := []byte("PNG_CONTENT_DETERMINISTIC")
	catMp3Bytes := []byte("MP3_AUDIO_STREAM")
	fontPngBytes := []byte("FONT_IMAGE_STATIC")

	_ = os.WriteFile(filepath.Join(mediaSrcDir, "cat.png"), catPngBytes, 0644)
	_ = os.WriteFile(filepath.Join(mediaSrcDir, "cat.mp3"), catMp3Bytes, 0644)
	_ = os.WriteFile(filepath.Join(mediaSrcDir, "_font.png"), fontPngBytes, 0644)

	req := helperCreateRichFixture(destDir, mediaSrcDir, true)
	// Добавляем дубликаты имен для проверки дедупликации
	req.MediaFiles = append(req.MediaFiles, "cat.png", "cat.mp3")

	res, err := export.ExecuteExport(req)
	if err != nil {
		t.Fatalf("ExecuteExport завершился с ошибкой: %v", err)
	}

	if res.Decks != 3 || res.Notes != 2 || res.Cards != 3 || res.NoteTypes != 2 {
		t.Errorf("неожиданные счетчики DTO: %+v", res)
	}
	if res.MediaFiles != 3 || res.MissingMedia != 1 {
		t.Errorf("неожиданные счетчики медиа (ожидалось 3 файла, 1 missing): %+v", res)
	}

	// 1. Проверка cards.jsonl: валидный JSON, ссылки на note_guid, разные колоды для одной note
	cardsData, err := os.ReadFile(filepath.Join(destDir, "cards.jsonl"))
	if err != nil {
		t.Fatalf("ошибка чтения cards.jsonl: %v", err)
	}
	cardLines := bytes.Split(bytes.TrimSpace(cardsData), []byte("\n"))
	if len(cardLines) != 3 {
		t.Fatalf("в cards.jsonl ожидалось 3 строки, получено: %d", len(cardLines))
	}
	multiCardsCount := 0
	for _, cl := range cardLines {
		var c export.CardDTO
		if err := json.Unmarshal(cl, &c); err != nil {
			t.Errorf("невалидная строка cards.jsonl: %v", err)
		}
		if c.NoteGUID == "note_guid_multi_cards" {
			multiCardsCount++
		}
	}
	if multiCardsCount != 2 {
		t.Errorf("ожидалось 2 карточки для note_guid_multi_cards, получено %d", multiCardsCount)
	}

	// 1.1. Проверка notes.jsonl: чистый HTML без экранирования \u003c
	notesData, err := os.ReadFile(filepath.Join(destDir, "notes.jsonl"))
	if err != nil {
		t.Fatalf("ошибка чтения notes.jsonl: %v", err)
	}
	if !bytes.Contains(notesData, []byte("<b>猫</b>")) {
		t.Errorf("notes.jsonl должен содержать чистый неэкранированный HTML <b>猫</b>")
	}
	if bytes.Contains(notesData, []byte(`\u003c`)) {
		t.Errorf("notes.jsonl не должен содержать HTML escape последовательностей \\u003c")
	}

	// 2. Проверка decks.json: parent_id hierarchy (root -> child -> grandchild)
	decksData, err := os.ReadFile(filepath.Join(destDir, "decks.json"))
	if err != nil {
		t.Fatalf("ошибка чтения decks.json: %v", err)
	}
	var decks []export.DeckDTO
	_ = json.Unmarshal(decksData, &decks)
	deckMap := make(map[string]export.DeckDTO)
	for _, d := range decks {
		deckMap[d.ID] = d
	}
	if deckMap["100"].ParentID != nil {
		t.Errorf("корневая колода 100 должна иметь ParentID == nil")
	}
	if deckMap["200"].ParentID == nil || *deckMap["200"].ParentID != "100" {
		t.Errorf("дочерняя колода 200 должна ссылаться на 100")
	}
	if deckMap["300"].ParentID == nil || *deckMap["300"].ParentID != "200" {
		t.Errorf("внучатая колода 300 должна ссылаться на 200")
	}

	// 3. Проверка note_types: семантика standard и cloze
	stdData, err := os.ReadFile(filepath.Join(destDir, "note_types", "model_std.json"))
	if err != nil {
		t.Fatalf("ошибка чтения model_std.json: %v", err)
	}
	var ntStd export.NoteTypeDTO
	_ = json.Unmarshal(stdData, &ntStd)
	if ntStd.Kind != "standard" {
		t.Errorf("ожидался kind standard для model_std, получено: %s", ntStd.Kind)
	}

	clozeData, err := os.ReadFile(filepath.Join(destDir, "note_types", "model_cloze.json"))
	if err != nil {
		t.Fatalf("ошибка чтения model_cloze.json: %v", err)
	}
	var ntCloze export.NoteTypeDTO
	_ = json.Unmarshal(clozeData, &ntCloze)
	if ntCloze.Kind != "cloze" {
		t.Errorf("ожидался kind cloze для model_cloze, получено: %s", ntCloze.Kind)
	}

	// 4. Проверка media.json: сверка с вычисленным SHA-256
	mediaData, err := os.ReadFile(filepath.Join(destDir, "media.json"))
	if err != nil {
		t.Fatalf("ошибка чтения media.json: %v", err)
	}
	var manifest export.MediaManifest
	_ = json.Unmarshal(mediaData, &manifest)

	h := sha256.New()
	h.Write(catPngBytes)
	expectedCatPngSHA := hex.EncodeToString(h.Sum(nil))

	foundExpectedSHA := false
	for _, mf := range manifest.Files {
		if mf.Name == "cat.png" {
			if mf.SHA256 != expectedCatPngSHA {
				t.Errorf("несовпадение SHA-256 для cat.png: ожидался %s, получен %s", expectedCatPngSHA, mf.SHA256)
			}
			foundExpectedSHA = true
		}
	}
	if !foundExpectedSHA {
		t.Errorf("файл cat.png не найден в манифесте media.json")
	}
}

func TestExport_RepeatExportMediaCleanup(t *testing.T) {
	tempDir := t.TempDir()
	mediaSrcDir := filepath.Join(tempDir, "source_media")
	destDir := filepath.Join(tempDir, "export_output")

	_ = os.MkdirAll(mediaSrcDir, 0755)
	_ = os.WriteFile(filepath.Join(mediaSrcDir, "cat.png"), []byte("CAT"), 0644)
	_ = os.WriteFile(filepath.Join(mediaSrcDir, "dog.png"), []byte("DOG"), 0644)

	// Экспорт 1: с включенным media в изначально пустой каталог
	req1 := helperCreateRichFixture(destDir, mediaSrcDir, true)
	if _, err := export.ExecuteExport(req1); err != nil {
		t.Fatalf("Экспорт 1 завершился с ошибкой: %v", err)
	}

	// Создаем пользовательский посторонний файл вне managed paths уже в существующем проекте
	userFile := filepath.Join(destDir, "user_notes.txt")
	_ = os.WriteFile(userFile, []byte("USER NOTES"), 0644)

	catPath := filepath.Join(destDir, "media", "cat.png")
	if _, err := os.Stat(catPath); os.IsNotExist(err) {
		t.Fatalf("cat.png должен существовать после первого экспорта")
	}

	// Экспорт 2: с выключенным media (include_media = false)
	req2 := helperCreateRichFixture(destDir, mediaSrcDir, false)
	if _, err := export.ExecuteExport(req2); err != nil {
		t.Fatalf("Экспорт 2 завершился с ошибкой: %v", err)
	}

	// Каталог media/ должен исчезнуть
	if _, err := os.Stat(filepath.Join(destDir, "media")); !os.IsNotExist(err) {
		t.Errorf("каталог media/ должен быть удален при повторном экспорте с include_media=false")
	}

	// Проверяем, что посторонний файл пользователя уцелел
	if data, err := os.ReadFile(userFile); err != nil || string(data) != "USER NOTES" {
		t.Errorf("пользовательский файл user_notes.txt был поврежден или удален!")
	}
}

func TestExport_UnsupportedSchemaVersion(t *testing.T) {
	tempDir := t.TempDir()
	markerPath := filepath.Join(tempDir, "crowdanki.json")
	meta := export.ManifestMeta{
		Format:        "crowdanki-v2",
		SchemaVersion: 999, // Неподдерживаемая будущая версия схемы
		RootDecks:     []string{"100"},
	}
	bytesMeta, _ := json.Marshal(meta)
	_ = os.WriteFile(markerPath, bytesMeta, 0644)

	req := helperCreateRichFixture(tempDir, "", false)
	_, err := export.ExecuteExport(req)
	if err == nil {
		t.Fatalf("ожидалась ошибка неподдерживаемой версии схемы, но экспорт прошел успешно")
	}
}

func TestExport_Determinism(t *testing.T) {
	tempDir := t.TempDir()
	mediaSrcDir := filepath.Join(tempDir, "source_media")
	destDir1 := filepath.Join(tempDir, "export1")
	destDir2 := filepath.Join(tempDir, "export2")

	_ = os.MkdirAll(mediaSrcDir, 0755)
	_ = os.WriteFile(filepath.Join(mediaSrcDir, "cat.png"), []byte("PNG_CONTENT"), 0644)
	_ = os.WriteFile(filepath.Join(mediaSrcDir, "cat.mp3"), []byte("MP3"), 0644)
	_ = os.WriteFile(filepath.Join(mediaSrcDir, "_font.png"), []byte("FONT"), 0644)

	req1 := helperCreateRichFixture(destDir1, mediaSrcDir, true)
	req2 := helperCreateRichFixture(destDir2, mediaSrcDir, true)

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
		filepath.Join("note_types", "model_std.json"),
		filepath.Join("note_types", "model_cloze.json"),
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

func TestExport_PathTraversalProtection(t *testing.T) {
	tempDir := t.TempDir()
	destDir := filepath.Join(tempDir, "export")
	mediaDir := filepath.Join(tempDir, "media")
	_ = os.MkdirAll(mediaDir, 0755)

	req := helperCreateRichFixture(destDir, mediaDir, true)
	req.MediaFiles = []string{"../dangerous.txt", "/etc/passwd", "..\\system.ini"}

	_, err := export.ExecuteExport(req)
	if err == nil {
		t.Fatalf("ожидалась ошибка path traversal, но экспорт прошел успешно")
	}
}
