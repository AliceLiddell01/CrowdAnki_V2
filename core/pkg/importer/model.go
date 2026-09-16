// Пакет importer реализует чтение, валидацию и построение детерминированного плана импорта CrowdAnki V2.
package importer

// PlanImportRequest содержит параметры запроса планирования импорта.
type PlanImportRequest struct {
	// SourceDir указывает путь к каталогу проекта CrowdAnki V2.
	SourceDir string `json:"source_dir"`
	// MediaDir указывает путь к каталогу медиафайлов целевой коллекции Anki.
	MediaDir string `json:"media_dir"`
	// IncludeMedia определяет, запрашивается ли импорт медиафайлов.
	IncludeMedia bool `json:"include_media"`
	// DestSnapshot содержит снимок текущего состояния управляемого scope коллекции Anki.
	DestSnapshot DestSnapshot `json:"dest_snapshot"`
}

// DestSnapshot описывает снимок целевой коллекции Anki, переданный Python-адаптером.
type DestSnapshot struct {
	Decks     []DestDeckDTO     `json:"decks"`
	NoteTypes []DestNoteTypeDTO `json:"note_types"`
	Notes     []DestNoteDTO     `json:"notes"`
	Cards     []DestCardDTO     `json:"cards"`
}

// DestDeckDTO представляет колоду целевой коллекции Anki.
type DestDeckDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// DestFieldDTO представляет поле типа заметки.
type DestFieldDTO struct {
	Name string `json:"name"`
	Ord  int    `json:"ord"`
}

// DestTemplateDTO представляет шаблон карточки типа заметки.
type DestTemplateDTO struct {
	Name  string `json:"name"`
	Ord   int    `json:"ord"`
	Qfmt  string `json:"qfmt"`
	Afmt  string `json:"afmt"`
	Bqfmt string `json:"bqfmt,omitempty"`
	Bafmt string `json:"bafmt,omitempty"`
}

// DestNoteTypeDTO представляет тип заметки в целевой коллекции.
type DestNoteTypeDTO struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Kind      string            `json:"kind"` // "standard" или "cloze"
	Sortf     int               `json:"sortf"`
	Fields    []DestFieldDTO    `json:"fields"`
	Templates []DestTemplateDTO `json:"templates"`
	CSS       string            `json:"css"`
}

// DestNoteDTO представляет заметку в целевой коллекции.
type DestNoteDTO struct {
	ID         string            `json:"id"`
	GUID       string            `json:"guid"`
	NoteType   string            `json:"note_type"`
	NoteTypeID string            `json:"note_type_id"`
	Fields     map[string]string `json:"fields"`
	Tags       []string          `json:"tags"`
}

// DestCardDTO представляет карточку в целевой коллекции.
type DestCardDTO struct {
	ID       string `json:"id"`
	NoteGUID string `json:"note_guid"`
	Ord      int    `json:"ord"`
	DeckName string `json:"deck_name"`
	InScope  bool   `json:"in_scope"`
}

// ConflictItem описывает обнаруженный конфликт, блокирующий импорт.
type ConflictItem struct {
	Category string `json:"category"` // "notetype", "media", "validation"
	Entity   string `json:"entity"`
	Message  string `json:"message"`
}

// PlanSummary содержит агрегированные числовые показатели проекта и планируемых изменений.
type PlanSummary struct {
	// Исходный проект
	TotalDecks      int   `json:"total_decks"`
	TotalNotes      int   `json:"total_notes"`
	TotalCards      int   `json:"total_cards"`
	TotalNoteTypes  int   `json:"total_note_types"`
	TotalMediaFiles int   `json:"total_media_files"`
	TotalMediaBytes int64 `json:"total_media_bytes"`

	// Изменения
	CreatedDecks     int `json:"created_decks"`
	DeletedDecks     int `json:"deleted_decks"`
	CreatedNotes     int `json:"created_notes"`
	UpdatedNotes     int `json:"updated_notes"`
	DeletedNotes     int `json:"deleted_notes"`
	CreatedCards     int `json:"created_cards"`
	MovedCards       int `json:"moved_cards"`
	DeletedCards     int `json:"deleted_cards"`
	CreatedNoteTypes int `json:"created_note_types"`
	UpdatedNoteTypes int `json:"updated_note_types"`
	AddedMedia       int `json:"added_media"`
	SameMedia        int `json:"same_media"`
	ConflictMedia    int `json:"conflict_media"`
	MissingMedia     int `json:"missing_media"`

	TotalConflicts int `json:"total_conflicts"`
}

// DeckOp представляет операцию над колодой.
type DeckOp struct {
	Action      string `json:"action"` // "create", "delete", "no-op"
	DeckID      string `json:"deck_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// NoteTypeOp представляет операцию над типом заметки.
type NoteTypeOp struct {
	Action    string            `json:"action"` // "create", "update", "conflict", "no-op"
	SourceID  string            `json:"source_id"`
	DestID    string            `json:"dest_id"`
	Name      string            `json:"name"`
	Kind      string            `json:"kind"`
	Sortf     int               `json:"sortf"`
	Fields    []DestFieldDTO    `json:"fields"`
	Templates []DestTemplateDTO `json:"templates"`
	CSS       string            `json:"css"`
	Details   string            `json:"details,omitempty"`
}

// NoteOp представляет операцию над заметкой.
type NoteOp struct {
	Action       string            `json:"action"` // "create", "update", "delete", "no-op"
	GUID         string            `json:"guid"`
	DestID       string            `json:"dest_id,omitempty"`
	NoteTypeName string            `json:"note_type_name"`
	Fields       map[string]string `json:"fields"`
	Tags         []string          `json:"tags"`
	FirstField   string            `json:"first_field"`
}

// CardOp представляет операцию над карточкой.
type CardOp struct {
	Action      string `json:"action"` // "create", "move", "delete", "no-op"
	DestID      string `json:"dest_id,omitempty"`
	NoteGUID    string `json:"note_guid"`
	Ord         int    `json:"ord"`
	OldDeck     string `json:"old_deck,omitempty"`
	NewDeck     string `json:"new_deck,omitempty"`
	NotePreview string `json:"note_preview,omitempty"`
}

// MediaOp представляет операцию над медиафайлом.
type MediaOp struct {
	Action       string `json:"action"` // "add", "same", "conflict", "missing"
	Name         string `json:"name"`
	SourcePath   string `json:"source_path"`
	Size         int64  `json:"size"`
	SourceSHA256 string `json:"source_sha256"`
	DestSHA256   string `json:"dest_sha256,omitempty"`
}

// ImportPlanResult представляет полный структурированный результат планирования импорта.
type ImportPlanResult struct {
	CanApply    bool           `json:"can_apply"`
	RootDecks   []string       `json:"root_decks"`
	Summary     PlanSummary    `json:"summary"`
	Conflicts   []ConflictItem `json:"conflicts"`
	Warnings    []string       `json:"warnings"`
	DeckOps     []DeckOp       `json:"deck_ops"`
	NoteTypeOps []NoteTypeOp   `json:"note_type_ops"`
	NoteOps     []NoteOp       `json:"note_ops"`
	CardOps     []CardOp       `json:"card_ops"`
	MediaOps    []MediaOp      `json:"media_ops"`
}
