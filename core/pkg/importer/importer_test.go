package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AliceLiddell01/CrowdAnki_V2/core/pkg/export"
)

// helper: создание временного валидного проекта CrowdAnki V2
func createTestProject(t *testing.T, modifyFn func(dir string)) string {
	t.Helper()
	dir := t.TempDir()

	// 1. crowdanki.json
	manifest := export.ManifestMeta{
		Format:        "crowdanki-v2",
		SchemaVersion: 1,
		RootDecks:     []string{"文法"},
	}
	writeJSON(t, filepath.Join(dir, "crowdanki.json"), manifest)

	// 2. decks.json
	parentID := "deck-root"
	decks := []export.DeckDTO{
		{
			ID:          "deck-root",
			ParentID:    nil,
			Name:        "文法",
			Description: "Корневая колода грамматики",
		},
		{
			ID:          "deck-n2",
			ParentID:    &parentID,
			Name:        "文法::N2",
			Description: "Уровень N2",
		},
	}
	writeJSON(t, filepath.Join(dir, "decks.json"), decks)

	// 3. note_types/
	ntDir := filepath.Join(dir, "note_types")
	if err := os.MkdirAll(ntDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	nt := export.NoteTypeDTO{
		ID:    "nt-basic",
		Name:  "Базовый",
		Kind:  "standard",
		Sortf: 0,
		Fields: []export.NoteFieldDTO{
			{Name: "Front", Ord: 0},
			{Name: "Back", Ord: 1},
		},
		Templates: []export.CardTemplateDTO{
			{Name: "Card 1", Ord: 0, Qfmt: "{{Front}}", Afmt: "{{FrontSide}}<hr>{{Back}}"},
		},
		CSS: ".card { font-family: arial; }",
	}
	writeJSON(t, filepath.Join(ntDir, "nt-basic.json"), nt)

	// 4. notes.jsonl
	notes := []export.NoteDTO{
		{
			GUID:       "guid-1",
			NoteTypeID: "nt-basic",
			NoteType:   "Базовый",
			Fields:     map[string]string{"Front": "食べる", "Back": "Есть, кушать"},
			Tags:       []string{"jlpt-n2", "verb"},
		},
	}
	writeNotesJSONL(t, filepath.Join(dir, "notes.jsonl"), notes)

	// 5. cards.jsonl
	cards := []export.CardDTO{
		{
			NoteGUID: "guid-1",
			CardID:   "card-src-1",
			Ord:      0,
			DeckID:   "deck-n2",
		},
	}
	writeCardsJSONL(t, filepath.Join(dir, "cards.jsonl"), cards)

	// 6. media.json & media/
	mediaDir := filepath.Join(dir, "media")
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	mediaContent := []byte("fake-audio-sample")
	audioHash := sha256Hex(mediaContent)
	if err := os.WriteFile(filepath.Join(mediaDir, "sample.mp3"), mediaContent, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	mediaManifest := export.MediaManifest{
		Included: true,
		Files: []export.MediaItem{
			{Name: "sample.mp3", Size: int64(len(mediaContent)), SHA256: audioHash},
		},
	}
	writeJSON(t, filepath.Join(dir, "media.json"), mediaManifest)

	if modifyFn != nil {
		modifyFn(dir)
	}

	return dir
}

func writeJSON(t *testing.T, path string, data any) {
	t.Helper()
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	if err := os.WriteFile(path, bytes, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}

func writeNotesJSONL(t *testing.T, path string, notes []export.NoteDTO) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer f.Close()
	for _, n := range notes {
		b, err := json.Marshal(n)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		f.Write(b)
		f.WriteString("\n")
	}
}

func writeCardsJSONL(t *testing.T, path string, cards []export.CardDTO) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer f.Close()
	for _, c := range cards {
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		f.Write(b)
		f.WriteString("\n")
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// 1. Тест No-op: исходный проект и целевой снимок логически равны
func TestImportPlanner_NoOp(t *testing.T) {
	projDir := createTestProject(t, nil)
	ankiMediaDir := t.TempDir()

	// Помещаем совпадающий медиафайл в целевую коллекцию
	mediaContent := []byte("fake-audio-sample")
	if err := os.WriteFile(filepath.Join(ankiMediaDir, "sample.mp3"), mediaContent, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	req := PlanImportRequest{
		SourceDir:    projDir,
		MediaDir:     ankiMediaDir,
		IncludeMedia: true,
		DestSnapshot: DestSnapshot{
			Decks: []DestDeckDTO{
				{ID: "100", Name: "文法", Description: "Корневая колода грамматики"},
				{ID: "101", Name: "文法::N2", Description: "Уровень N2"},
			},
			NoteTypes: []DestNoteTypeDTO{
				{
					ID:    "200",
					Name:  "Базовый",
					Kind:  "standard",
					Sortf: 0,
					Fields: []DestFieldDTO{
						{Name: "Front", Ord: 0},
						{Name: "Back", Ord: 1},
					},
					Templates: []DestTemplateDTO{
						{Name: "Card 1", Ord: 0, Qfmt: "{{Front}}", Afmt: "{{FrontSide}}<hr>{{Back}}"},
					},
					CSS: ".card { font-family: arial; }",
				},
			},
			Notes: []DestNoteDTO{
				{
					ID:       "300",
					GUID:     "guid-1",
					NoteType: "Базовый",
					Fields:   map[string]string{"Front": "食べる", "Back": "Есть, кушать"},
					Tags:     []string{"jlpt-n2", "verb"},
				},
			},
			Cards: []DestCardDTO{
				{
					ID:       "400",
					NoteGUID: "guid-1",
					Ord:      0,
					DeckName: "文法::N2",
					InScope:  true,
				},
			},
		},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if !plan.CanApply {
		t.Fatalf("Expected CanApply=true, got false. Conflicts: %+v", plan.Conflicts)
	}

	if plan.Summary.CreatedDecks != 0 || plan.Summary.DeletedDecks != 0 ||
		plan.Summary.CreatedNotes != 0 || plan.Summary.UpdatedNotes != 0 || plan.Summary.DeletedNotes != 0 ||
		plan.Summary.CreatedCards != 0 || plan.Summary.MovedCards != 0 || plan.Summary.DeletedCards != 0 ||
		plan.Summary.CreatedNoteTypes != 0 || plan.Summary.UpdatedNoteTypes != 0 ||
		plan.Summary.AddedMedia != 0 || plan.Summary.ConflictMedia != 0 || plan.Summary.TotalConflicts != 0 {
		t.Fatalf("Expected all zero mutations in summary, got: %+v", plan.Summary)
	}

	if plan.Summary.SameMedia != 1 {
		t.Fatalf("Expected SameMedia=1, got %d", plan.Summary.SameMedia)
	}
}

// 2. Тест Создание: новая колода, новая заметка, карточки, тип заметки
func TestImportPlanner_Creation(t *testing.T) {
	projDir := createTestProject(t, nil)
	ankiMediaDir := t.TempDir()

	// Пустая коллекция
	req := PlanImportRequest{
		SourceDir:    projDir,
		MediaDir:     ankiMediaDir,
		IncludeMedia: false,
		DestSnapshot: DestSnapshot{},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if !plan.CanApply {
		t.Fatalf("Expected CanApply=true, got false")
	}

	if plan.Summary.CreatedDecks != 2 {
		t.Errorf("Expected 2 created decks, got %d", plan.Summary.CreatedDecks)
	}
	if plan.Summary.CreatedNoteTypes != 1 {
		t.Errorf("Expected 1 created note type, got %d", plan.Summary.CreatedNoteTypes)
	}
	if plan.Summary.CreatedNotes != 1 {
		t.Errorf("Expected 1 created note, got %d", plan.Summary.CreatedNotes)
	}
	if plan.Summary.CreatedCards != 1 {
		t.Errorf("Expected 1 created card, got %d", plan.Summary.CreatedCards)
	}
}

// 3. Тест Обновление Note: изменено поле (update, а не delete+create)
func TestImportPlanner_NoteFieldUpdate(t *testing.T) {
	projDir := createTestProject(t, nil)

	req := PlanImportRequest{
		SourceDir: projDir,
		DestSnapshot: DestSnapshot{
			Decks: []DestDeckDTO{
				{ID: "100", Name: "文法"},
				{ID: "101", Name: "文法::N2"},
			},
			NoteTypes: []DestNoteTypeDTO{
				{
					Name:   "Базовый",
					Fields: []DestFieldDTO{{Name: "Front", Ord: 0}, {Name: "Back", Ord: 1}},
					Templates: []DestTemplateDTO{
						{Name: "Card 1", Ord: 0, Qfmt: "{{Front}}", Afmt: "{{FrontSide}}<hr>{{Back}}"},
					},
					CSS: ".card { font-family: arial; }",
				},
			},
			Notes: []DestNoteDTO{
				{
					ID:       "300",
					GUID:     "guid-1",
					NoteType: "Базовый",
					Fields:   map[string]string{"Front": "食べる", "Back": "Старый перевод"},
					Tags:     []string{"jlpt-n2", "verb"},
				},
			},
			Cards: []DestCardDTO{
				{
					ID:       "400",
					NoteGUID: "guid-1",
					Ord:      0,
					DeckName: "文法::N2",
					InScope:  true,
				},
			},
		},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if plan.Summary.UpdatedNotes != 1 {
		t.Errorf("Expected UpdatedNotes=1, got %d", plan.Summary.UpdatedNotes)
	}
	if plan.Summary.DeletedNotes != 0 || plan.Summary.CreatedNotes != 0 {
		t.Errorf("Expected 0 deleted and 0 created notes, got del=%d, cr=%d",
			plan.Summary.DeletedNotes, plan.Summary.CreatedNotes)
	}
}

// 4. Тест Tags: изменение тегов классифицируется как update заметки
func TestImportPlanner_NoteTagsUpdate(t *testing.T) {
	projDir := createTestProject(t, nil)

	req := PlanImportRequest{
		SourceDir: projDir,
		DestSnapshot: DestSnapshot{
			Decks: []DestDeckDTO{
				{ID: "100", Name: "文法"},
				{ID: "101", Name: "文法::N2"},
			},
			NoteTypes: []DestNoteTypeDTO{
				{
					Name:   "Базовый",
					Fields: []DestFieldDTO{{Name: "Front", Ord: 0}, {Name: "Back", Ord: 1}},
					Templates: []DestTemplateDTO{
						{Name: "Card 1", Ord: 0, Qfmt: "{{Front}}", Afmt: "{{FrontSide}}<hr>{{Back}}"},
					},
					CSS: ".card { font-family: arial; }",
				},
			},
			Notes: []DestNoteDTO{
				{
					ID:       "300",
					GUID:     "guid-1",
					NoteType: "Базовый",
					Fields:   map[string]string{"Front": "食べる", "Back": "Есть, кушать"},
					Tags:     []string{"old-tag"}, // отличающиеся теги
				},
			},
			Cards: []DestCardDTO{
				{
					ID:       "400",
					NoteGUID: "guid-1",
					Ord:      0,
					DeckName: "文法::N2",
					InScope:  true,
				},
			},
		},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if plan.Summary.UpdatedNotes != 1 {
		t.Errorf("Expected UpdatedNotes=1, got %d", plan.Summary.UpdatedNotes)
	}
}

// 5. Тест Card move: (note_guid, ord) совпадает, но колода отличается -> move
func TestImportPlanner_CardMove(t *testing.T) {
	projDir := createTestProject(t, nil)

	req := PlanImportRequest{
		SourceDir: projDir,
		DestSnapshot: DestSnapshot{
			Decks: []DestDeckDTO{
				{ID: "100", Name: "文法"},
				{ID: "101", Name: "文法::N2"},
			},
			Notes: []DestNoteDTO{
				{
					ID:       "300",
					GUID:     "guid-1",
					NoteType: "Базовый",
					Fields:   map[string]string{"Front": "食べる", "Back": "Есть, кушать"},
					Tags:     []string{"jlpt-n2", "verb"},
				},
			},
			Cards: []DestCardDTO{
				{
					ID:       "400",
					NoteGUID: "guid-1",
					Ord:      0,
					DeckName: "文法", // В целевой колода корень, в источнике 文法::N2
					InScope:  true,
				},
			},
		},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if plan.Summary.MovedCards != 1 {
		t.Errorf("Expected MovedCards=1, got %d", plan.Summary.MovedCards)
	}

	foundMove := false
	for _, cop := range plan.CardOps {
		if cop.Action == "move" && cop.NoteGUID == "guid-1" && cop.OldDeck == "文法" && cop.NewDeck == "文法::N2" {
			foundMove = true
			break
		}
	}
	if !foundMove {
		t.Errorf("Expected move card op not found in plan: %+v", plan.CardOps)
	}
}

// 6. Тест Card deletion: карточка в scope, но отсутствует в источнике
func TestImportPlanner_CardDeletion(t *testing.T) {
	projDir := createTestProject(t, nil)

	req := PlanImportRequest{
		SourceDir: projDir,
		DestSnapshot: DestSnapshot{
			Decks: []DestDeckDTO{
				{ID: "100", Name: "文法"},
				{ID: "101", Name: "文法::N2"},
			},
			Notes: []DestNoteDTO{
				{
					ID:       "300",
					GUID:     "guid-1",
					NoteType: "Базовый",
					Fields:   map[string]string{"Front": "食べる", "Back": "Есть, кушать"},
					Tags:     []string{"jlpt-n2", "verb"},
				},
			},
			Cards: []DestCardDTO{
				{ID: "400", NoteGUID: "guid-1", Ord: 0, DeckName: "文法::N2", InScope: true},
				{ID: "401", NoteGUID: "guid-1", Ord: 1, DeckName: "文法::N2", InScope: true}, // Лишняя карточка ord=1
			},
		},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if plan.Summary.DeletedCards != 1 {
		t.Errorf("Expected DeletedCards=1, got %d", plan.Summary.DeletedCards)
	}
}

// 7. Тест Note deletion: заметка исчезла из источника и все её карточки в scope -> полное удаление
func TestImportPlanner_NoteFullDeletion(t *testing.T) {
	projDir := createTestProject(t, nil)

	req := PlanImportRequest{
		SourceDir: projDir,
		DestSnapshot: DestSnapshot{
			Decks: []DestDeckDTO{
				{ID: "100", Name: "文法"},
				{ID: "101", Name: "文法::N2"},
			},
			Notes: []DestNoteDTO{
				{
					ID:       "300",
					GUID:     "guid-1",
					NoteType: "Базовый",
					Fields:   map[string]string{"Front": "食べる", "Back": "Есть, кушать"},
					Tags:     []string{"jlpt-n2", "verb"},
				},
				{
					ID:       "301",
					GUID:     "guid-obsolete",
					NoteType: "Базовый",
					Fields:   map[string]string{"Front": "Устаревшая", "Back": "Удаленная"},
				},
			},
			Cards: []DestCardDTO{
				{ID: "400", NoteGUID: "guid-1", Ord: 0, DeckName: "文法::N2", InScope: true},
				{ID: "402", NoteGUID: "guid-obsolete", Ord: 0, DeckName: "文法::N2", InScope: true},
			},
		},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if plan.Summary.DeletedNotes != 1 {
		t.Errorf("Expected DeletedNotes=1, got %d", plan.Summary.DeletedNotes)
	}
	if plan.Summary.DeletedCards != 1 {
		t.Errorf("Expected DeletedCards=1, got %d", plan.Summary.DeletedCards)
	}
}

// 8. Обязательный регрессионный тест: Out-of-scope card protection
// Заметка исчезла из источника, но имеет карточку вне import scope.
// План: удаляет только карточку в scope, НЕ удаляет Note, НЕ затрагивает карточку вне scope!
func TestImportPlanner_OutOfScopeCardProtection(t *testing.T) {
	projDir := createTestProject(t, nil)

	req := PlanImportRequest{
		SourceDir: projDir,
		DestSnapshot: DestSnapshot{
			Decks: []DestDeckDTO{
				{ID: "100", Name: "文法"},
				{ID: "101", Name: "文法::N2"},
				{ID: "999", Name: "ДругаяКолода"}, // вне scope
			},
			Notes: []DestNoteDTO{
				{
					ID:       "300",
					GUID:     "guid-1",
					NoteType: "Базовый",
					Fields:   map[string]string{"Front": "食べる", "Back": "Есть, кушать"},
					Tags:     []string{"jlpt-n2", "verb"},
				},
				{
					ID:       "302",
					GUID:     "guid-multi-deck",
					NoteType: "Базовый",
					Fields:   map[string]string{"Front": "Слово с карточками в 2 колодах", "Back": "Определение"},
				},
			},
			Cards: []DestCardDTO{
				{ID: "400", NoteGUID: "guid-1", Ord: 0, DeckName: "文法::N2", InScope: true},
				// У заметки guid-multi-deck одна карточка в scope, вторая вне scope!
				{ID: "410", NoteGUID: "guid-multi-deck", Ord: 0, DeckName: "文法::N2", InScope: true},
				{ID: "411", NoteGUID: "guid-multi-deck", Ord: 1, DeckName: "ДругаяКолода", InScope: false},
			},
		},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	// 1. Заметка НЕ должна удаляться, так как карточка 411 вне scope
	if plan.Summary.DeletedNotes != 0 {
		t.Fatalf("CRITICAL: Out-of-scope protection violated! DeletedNotes=%d (expected 0)", plan.Summary.DeletedNotes)
	}

	// 2. Должна быть удалена только карточка 410 в scope
	if plan.Summary.DeletedCards != 1 {
		t.Fatalf("Expected exactly 1 deleted card, got %d", plan.Summary.DeletedCards)
	}

	for _, cop := range plan.CardOps {
		if cop.DestID == "411" {
			t.Fatalf("CRITICAL: Out-of-scope card 411 was touched in CardOps: %+v", cop)
		}
		if cop.Action == "delete" && cop.DestID != "410" {
			t.Fatalf("Unexpected card deleted: %+v", cop)
		}
	}

	for _, nop := range plan.NoteOps {
		if nop.GUID == "guid-multi-deck" && nop.Action == "delete" {
			t.Fatalf("CRITICAL: Note guid-multi-deck was scheduled for deletion!")
		}
	}
}

// 9. Тест Deck deletion: дочерняя колода в scope удаляется
func TestImportPlanner_DeckDeletion(t *testing.T) {
	projDir := createTestProject(t, nil)

	req := PlanImportRequest{
		SourceDir: projDir,
		DestSnapshot: DestSnapshot{
			Decks: []DestDeckDTO{
				{ID: "100", Name: "文法"},
				{ID: "101", Name: "文法::N2"},
				{ID: "102", Name: "文法::N2::УстаревшийУрок"}, // дочерняя в scope, которой нет в источнике
			},
			Notes: []DestNoteDTO{
				{ID: "300", GUID: "guid-1", NoteType: "Базовый", Fields: map[string]string{"Front": "食べる", "Back": "Есть, кушать"}},
			},
			Cards: []DestCardDTO{
				{ID: "400", NoteGUID: "guid-1", Ord: 0, DeckName: "文法::N2", InScope: true},
			},
		},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if plan.Summary.DeletedDecks != 1 {
		t.Errorf("Expected DeletedDecks=1, got %d", plan.Summary.DeletedDecks)
	}

	found := false
	for _, dop := range plan.DeckOps {
		if dop.Action == "delete" && dop.Name == "文法::N2::УстаревшийУрок" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected deck delete op not found: %+v", plan.DeckOps)
	}
}

// 10. Тест Deck outside scope: соседняя root колода не попадает в plan
func TestImportPlanner_DeckOutsideScope(t *testing.T) {
	projDir := createTestProject(t, nil)

	req := PlanImportRequest{
		SourceDir: projDir,
		DestSnapshot: DestSnapshot{
			Decks: []DestDeckDTO{
				{ID: "100", Name: "文法"},
				{ID: "101", Name: "文法::N2"},
				{ID: "500", Name: "Английский"}, // соседняя root колода
			},
			Notes: []DestNoteDTO{
				{ID: "300", GUID: "guid-1", NoteType: "Базовый", Fields: map[string]string{"Front": "食べる", "Back": "Есть, кушать"}},
			},
			Cards: []DestCardDTO{
				{ID: "400", NoteGUID: "guid-1", Ord: 0, DeckName: "文法::N2", InScope: true},
			},
		},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	for _, dop := range plan.DeckOps {
		if dop.Name == "Английский" {
			t.Fatalf("Deck outside scope 'Английский' should not be in plan: %+v", dop)
		}
	}
}

// 11. Тест Source IDs differ: одинаковая структура с другими локальными ID не считается чужой
func TestImportPlanner_SourceIDsDiffer(t *testing.T) {
	projDir := createTestProject(t, nil)

	req := PlanImportRequest{
		SourceDir: projDir,
		DestSnapshot: DestSnapshot{
			Decks: []DestDeckDTO{
				{ID: "local-deck-id-999", Name: "文法"},
				{ID: "local-deck-id-888", Name: "文法::N2"},
			},
			NoteTypes: []DestNoteTypeDTO{
				{
					ID:     "local-nt-777",
					Name:   "Базовый",
					Fields: []DestFieldDTO{{Name: "Front", Ord: 0}, {Name: "Back", Ord: 1}},
					Templates: []DestTemplateDTO{
						{Name: "Card 1", Ord: 0, Qfmt: "{{Front}}", Afmt: "{{FrontSide}}<hr>{{Back}}"},
					},
					CSS: ".card { font-family: arial; }",
				},
			},
			Notes: []DestNoteDTO{
				{
					ID:       "local-note-111",
					GUID:     "guid-1", // совпадает GUID
					NoteType: "Базовый",
					Fields:   map[string]string{"Front": "食べる", "Back": "Есть, кушать"},
					Tags:     []string{"jlpt-n2", "verb"},
				},
			},
			Cards: []DestCardDTO{
				{
					ID:       "local-card-222",
					NoteGUID: "guid-1",
					Ord:      0,
					DeckName: "文法::N2",
					InScope:  true,
				},
			},
		},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if plan.Summary.CreatedDecks != 0 || plan.Summary.CreatedNotes != 0 || plan.Summary.CreatedCards != 0 {
		t.Errorf("Expected 0 creations despite different local IDs, got: %+v", plan.Summary)
	}
}

// 12. Тест Note type CSS update
func TestImportPlanner_NoteTypeCSSUpdate(t *testing.T) {
	projDir := createTestProject(t, nil)

	req := PlanImportRequest{
		SourceDir: projDir,
		DestSnapshot: DestSnapshot{
			NoteTypes: []DestNoteTypeDTO{
				{
					Name:   "Базовый",
					Fields: []DestFieldDTO{{Name: "Front", Ord: 0}, {Name: "Back", Ord: 1}},
					Templates: []DestTemplateDTO{
						{Name: "Card 1", Ord: 0, Qfmt: "{{Front}}", Afmt: "{{FrontSide}}<hr>{{Back}}"},
					},
					CSS: ".card { font-size: 14px; }", // Другой CSS
				},
			},
		},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if plan.Summary.UpdatedNoteTypes != 1 {
		t.Errorf("Expected UpdatedNoteTypes=1, got %d", plan.Summary.UpdatedNoteTypes)
	}
}

// 13. Тест Template update
func TestImportPlanner_NoteTypeTemplateUpdate(t *testing.T) {
	projDir := createTestProject(t, nil)

	req := PlanImportRequest{
		SourceDir: projDir,
		DestSnapshot: DestSnapshot{
			NoteTypes: []DestNoteTypeDTO{
				{
					Name:   "Базовый",
					Fields: []DestFieldDTO{{Name: "Front", Ord: 0}, {Name: "Back", Ord: 1}},
					Templates: []DestTemplateDTO{
						{Name: "Card 1", Ord: 0, Qfmt: "{{Front}}", Afmt: "Old template"}, // Другой Afmt
					},
					CSS: ".card { font-family: arial; }",
				},
			},
		},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if plan.Summary.UpdatedNoteTypes != 1 {
		t.Errorf("Expected UpdatedNoteTypes=1, got %d", plan.Summary.UpdatedNoteTypes)
	}
}

// 14. Тест Ambiguous field change: удаление одного поля + появление другого -> конфликт
func TestImportPlanner_AmbiguousFieldChangeConflict(t *testing.T) {
	projDir := createTestProject(t, nil)

	req := PlanImportRequest{
		SourceDir: projDir,
		DestSnapshot: DestSnapshot{
			NoteTypes: []DestNoteTypeDTO{
				{
					Name: "Базовый",
					Fields: []DestFieldDTO{
						{Name: "Front", Ord: 0},
						{Name: "СтароеПоле", Ord: 1}, // в источнике Back
					},
					Templates: []DestTemplateDTO{
						{Name: "Card 1", Ord: 0, Qfmt: "{{Front}}", Afmt: "{{FrontSide}}<hr>{{Back}}"},
					},
					CSS: ".card { font-family: arial; }",
				},
			},
		},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if plan.CanApply {
		t.Errorf("Expected CanApply=false on ambiguous field change, got true")
	}
	if plan.Summary.TotalConflicts == 0 {
		t.Errorf("Expected TotalConflicts > 0, got 0")
	}
}

// 15. Тест Determinism: одинаковые source/snapshot -> идентичный ImportPlan
func TestImportPlanner_Determinism(t *testing.T) {
	projDir := createTestProject(t, nil)

	req := PlanImportRequest{
		SourceDir: projDir,
		DestSnapshot: DestSnapshot{
			Decks: []DestDeckDTO{
				{ID: "101", Name: "文法::N2"},
				{ID: "100", Name: "文法"},
			},
		},
	}

	plan1, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport 1 failed: %v", err)
	}
	plan2, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport 2 failed: %v", err)
	}

	b1, _ := json.Marshal(plan1)
	b2, _ := json.Marshal(plan2)

	if string(b1) != string(b2) {
		t.Fatalf("Plans differ between runs!\nPlan 1: %s\nPlan 2: %s", string(b1), string(b2))
	}
}

// 16. Тест Invalid references: карточка с неизвестным note GUID -> validation error
func TestImportPlanner_InvalidReferences(t *testing.T) {
	projDir := createTestProject(t, func(dir string) {
		// Добавляем карточку с несуществующим GUID
		badCard := export.CardDTO{
			NoteGUID: "non-existent-guid",
			CardID:   "bad-card",
			Ord:      0,
			DeckID:   "deck-n2",
		}
		writeCardsJSONL(t, filepath.Join(dir, "cards.jsonl"), []export.CardDTO{badCard})
	})

	req := PlanImportRequest{
		SourceDir: projDir,
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport unexpected error: %v", err)
	}
	if plan.CanApply {
		t.Errorf("Expected CanApply=false on invalid ref, got true")
	}
	if len(plan.Conflicts) == 0 {
		t.Errorf("Expected validation conflicts, got 0")
	}
}

// 17. Тест Cyclic decks: цикл parent refs -> validation error
func TestImportPlanner_CyclicDecks(t *testing.T) {
	projDir := createTestProject(t, func(dir string) {
		idA := "deck-a"
		idB := "deck-b"
		cyclicDecks := []export.DeckDTO{
			{ID: idA, ParentID: &idB, Name: "文法"},
			{ID: idB, ParentID: &idA, Name: "文法::N2"},
		}
		writeJSON(t, filepath.Join(dir, "decks.json"), cyclicDecks)
	})

	req := PlanImportRequest{
		SourceDir: projDir,
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport unexpected error: %v", err)
	}
	if plan.CanApply {
		t.Errorf("Expected CanApply=false on cyclic decks, got true")
	}
}

// 18. Медиа-тесты
func TestImportPlanner_MediaScenarios(t *testing.T) {
	t.Run("NewMedia_Add", func(t *testing.T) {
		projDir := createTestProject(t, nil)
		ankiMediaDir := t.TempDir() // Пустой каталог -> файл должен добавиться

		req := PlanImportRequest{
			SourceDir:    projDir,
			MediaDir:     ankiMediaDir,
			IncludeMedia: true,
		}
		plan, err := PlanImport(req)
		if err != nil {
			t.Fatalf("PlanImport failed: %v", err)
		}
		if plan.Summary.AddedMedia != 1 {
			t.Errorf("Expected AddedMedia=1, got %d", plan.Summary.AddedMedia)
		}
	})

	t.Run("SameMedia_NoOp", func(t *testing.T) {
		projDir := createTestProject(t, nil)
		ankiMediaDir := t.TempDir()
		// Создаем файл с тем же хэшем
		os.WriteFile(filepath.Join(ankiMediaDir, "sample.mp3"), []byte("fake-audio-sample"), 0644)

		req := PlanImportRequest{
			SourceDir:    projDir,
			MediaDir:     ankiMediaDir,
			IncludeMedia: true,
		}
		plan, err := PlanImport(req)
		if err != nil {
			t.Fatalf("PlanImport failed: %v", err)
		}
		if plan.Summary.SameMedia != 1 {
			t.Errorf("Expected SameMedia=1, got %d", plan.Summary.SameMedia)
		}
	})

	t.Run("ConflictingMedia_Conflict", func(t *testing.T) {
		projDir := createTestProject(t, nil)
		ankiMediaDir := t.TempDir()
		// Создаем файл с другим содержанием и хэшем
		os.WriteFile(filepath.Join(ankiMediaDir, "sample.mp3"), []byte("different-audio-content"), 0644)

		req := PlanImportRequest{
			SourceDir:    projDir,
			MediaDir:     ankiMediaDir,
			IncludeMedia: true,
		}
		plan, err := PlanImport(req)
		if err != nil {
			t.Fatalf("PlanImport failed: %v", err)
		}
		if plan.Summary.ConflictMedia != 1 {
			t.Errorf("Expected ConflictMedia=1, got %d", plan.Summary.ConflictMedia)
		}
		if plan.CanApply {
			t.Errorf("Expected CanApply=false due to media conflict, got true")
		}
	})

	t.Run("ConflictingMedia_StrategySkip", func(t *testing.T) {
		projDir := createTestProject(t, nil)
		ankiMediaDir := t.TempDir()
		os.WriteFile(filepath.Join(ankiMediaDir, "sample.mp3"), []byte("different-audio-content"), 0644)

		req := PlanImportRequest{
			SourceDir:             projDir,
			MediaDir:              ankiMediaDir,
			IncludeMedia:          true,
			MediaConflictStrategy: "skip",
		}
		plan, err := PlanImport(req)
		if err != nil {
			t.Fatalf("PlanImport failed: %v", err)
		}
		if plan.Summary.ConflictMedia != 1 {
			t.Errorf("Expected ConflictMedia=1, got %d", plan.Summary.ConflictMedia)
		}
		if !plan.CanApply {
			t.Errorf("Expected CanApply=true when strategy=skip, got false. Conflicts: %+v", plan.Conflicts)
		}
		if len(plan.Warnings) != 1 {
			t.Errorf("Expected 1 warning for skipped media, got %d", len(plan.Warnings))
		}
		if len(plan.MediaOps) != 1 || plan.MediaOps[0].Action != "skip" {
			t.Errorf("Expected MediaOp action 'skip', got %+v", plan.MediaOps)
		}
	})

	t.Run("ConflictingMedia_StrategyOverwrite", func(t *testing.T) {
		projDir := createTestProject(t, nil)
		ankiMediaDir := t.TempDir()
		os.WriteFile(filepath.Join(ankiMediaDir, "sample.mp3"), []byte("different-audio-content"), 0644)

		req := PlanImportRequest{
			SourceDir:             projDir,
			MediaDir:              ankiMediaDir,
			IncludeMedia:          true,
			MediaConflictStrategy: "overwrite",
		}
		plan, err := PlanImport(req)
		if err != nil {
			t.Fatalf("PlanImport failed: %v", err)
		}
		if plan.Summary.ConflictMedia != 1 {
			t.Errorf("Expected ConflictMedia=1, got %d", plan.Summary.ConflictMedia)
		}
		if !plan.CanApply {
			t.Errorf("Expected CanApply=true when strategy=overwrite, got false. Conflicts: %+v", plan.Conflicts)
		}
		if len(plan.Warnings) != 1 {
			t.Errorf("Expected 1 warning for overwritten media, got %d", len(plan.Warnings))
		}
		if len(plan.MediaOps) != 1 || plan.MediaOps[0].Action != "overwrite" {
			t.Errorf("Expected MediaOp action 'overwrite', got %+v", plan.MediaOps)
		}
	})

	t.Run("MediaDisabled_BypassesConflict", func(t *testing.T) {
		projDir := createTestProject(t, nil)
		ankiMediaDir := t.TempDir()
		// Создаем конфликтный файл
		os.WriteFile(filepath.Join(ankiMediaDir, "sample.mp3"), []byte("different-audio-content"), 0644)

		// Но отключаем импорт медиа
		req := PlanImportRequest{
			SourceDir:    projDir,
			MediaDir:     ankiMediaDir,
			IncludeMedia: false,
		}
		plan, err := PlanImport(req)
		if err != nil {
			t.Fatalf("PlanImport failed: %v", err)
		}
		if !plan.CanApply {
			t.Errorf("Expected CanApply=true when media disabled, got false. Conflicts: %+v", plan.Conflicts)
		}
	})

	t.Run("MissingSourceMedia", func(t *testing.T) {
		projDir := createTestProject(t, func(dir string) {
			// Удаляем физический файл media/sample.mp3
			os.Remove(filepath.Join(dir, "media", "sample.mp3"))
		})

		req := PlanImportRequest{
			SourceDir:    projDir,
			IncludeMedia: true,
		}
		plan, err := PlanImport(req)
		if err != nil {
			t.Fatalf("PlanImport failed: %v", err)
		}
		if plan.Summary.MissingMedia != 1 {
			t.Errorf("Expected MissingMedia=1, got %d", plan.Summary.MissingMedia)
		}
		if plan.CanApply {
			t.Errorf("Expected CanApply=false when source media missing, got true")
		}
	})

	t.Run("PathTraversalProtection", func(t *testing.T) {
		projDir := createTestProject(t, func(dir string) {
			// Внедряем файл с выходом за пределы каталога
			badManifest := export.MediaManifest{
				Included: true,
				Files: []export.MediaItem{
					{Name: "../../windows/system32/cmd.exe", Size: 10, SHA256: "abc"},
				},
			}
			writeJSON(t, filepath.Join(dir, "media.json"), badManifest)
		})

		req := PlanImportRequest{
			SourceDir: projDir,
		}
		plan, err := PlanImport(req)
		if err != nil {
			t.Fatalf("PlanImport unexpected error: %v", err)
		}
		if plan.CanApply {
			t.Errorf("Expected CanApply=false on path traversal, got true")
		}
	})
}

// 19. Сквозной round-trip fixture test
func TestImportPlanner_RoundTripFixture(t *testing.T) {
	projDir := createTestProject(t, nil)
	ankiMediaDir := t.TempDir()

	// Исходный снимок старых данных
	destSnap := DestSnapshot{
		Decks: []DestDeckDTO{
			{ID: "1", Name: "文法"},
			{ID: "2", Name: "文法::СтарыйУрок"},
		},
		NoteTypes: []DestNoteTypeDTO{
			{
				ID:     "10",
				Name:   "Базовый",
				Kind:   "standard",
				Fields: []DestFieldDTO{{Name: "Front", Ord: 0}, {Name: "Back", Ord: 1}},
				Templates: []DestTemplateDTO{
					{Name: "Card 1", Ord: 0, Qfmt: "{{Front}}", Afmt: "{{FrontSide}}<hr>{{Back}}"},
				},
				CSS: ".card { font-family: arial; }",
			},
		},
		Notes: []DestNoteDTO{
			{
				ID:       "100",
				GUID:     "guid-old",
				NoteType: "Базовый",
				Fields:   map[string]string{"Front": "Старая карточка", "Back": "Удалить"},
			},
		},
		Cards: []DestCardDTO{
			{
				ID:       "200",
				NoteGUID: "guid-old",
				Ord:      0,
				DeckName: "文法::СтарыйУрок",
				InScope:  true,
			},
		},
	}

	req := PlanImportRequest{
		SourceDir:    projDir,
		MediaDir:     ankiMediaDir,
		IncludeMedia: false,
		DestSnapshot: destSnap,
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if !plan.CanApply {
		t.Fatalf("Expected CanApply=true, got false. Conflicts: %+v", plan.Conflicts)
	}

	// Проверяем все 4 типа операций:
	// Create: deck 文法::N2, note guid-1, card guid-1:0
	if plan.Summary.CreatedDecks != 1 {
		t.Errorf("Expected CreatedDecks=1, got %d", plan.Summary.CreatedDecks)
	}
	if plan.Summary.CreatedNotes != 1 {
		t.Errorf("Expected CreatedNotes=1, got %d", plan.Summary.CreatedNotes)
	}
	if plan.Summary.CreatedCards != 1 {
		t.Errorf("Expected CreatedCards=1, got %d", plan.Summary.CreatedCards)
	}

	// Delete: deck 文法::СтарыйУрок, note guid-old, card guid-old:0
	if plan.Summary.DeletedDecks != 1 {
		t.Errorf("Expected DeletedDecks=1, got %d", plan.Summary.DeletedDecks)
	}
	if plan.Summary.DeletedNotes != 1 {
		t.Errorf("Expected DeletedNotes=1, got %d", plan.Summary.DeletedNotes)
	}
	if plan.Summary.DeletedCards != 1 {
		t.Errorf("Expected DeletedCards=1, got %d", plan.Summary.DeletedCards)
	}

	// Проверяем наличие предупреждений
}

func TestImportPlanner_RootDeckIDResolution(t *testing.T) {
	// Проект, где в crowdanki.json в root_decks сохранен ID колоды вместо ее имени
	projDir := createTestProject(t, func(dir string) {
		manifest := export.ManifestMeta{
			Format:        "crowdanki-v2",
			SchemaVersion: 1,
			RootDecks:     []string{"deck-root"}, // ID вместо "文法"
		}
		writeJSON(t, filepath.Join(dir, "crowdanki.json"), manifest)
	})

	req := PlanImportRequest{
		SourceDir:    projDir,
		MediaDir:     t.TempDir(),
		IncludeMedia: false,
		DestSnapshot: DestSnapshot{},
	}

	plan, err := PlanImport(req)
	if err != nil {
		t.Fatalf("PlanImport failed: %v", err)
	}

	if !plan.CanApply {
		t.Fatalf("Ожидался CanApply=true при резолвинге ID корневой колоды, получены конфликты: %+v", plan.Conflicts)
	}

	if len(plan.RootDecks) != 1 || plan.RootDecks[0] != "文法" {
		t.Errorf("Ожидалось разрешение ID 'deck-root' в имя '文法', получено: %+v", plan.RootDecks)
	}
}
