// Пакет export реализует логику экспорта коллекции CrowdAnki V2.
package export

// ExportRequest содержит входные параметры и данные для выполнения экспорта.
type ExportRequest struct {
	// DestinationDir указывает целевой каталог для сохранения экспорта.
	DestinationDir string `json:"destination_dir"`
	// MediaDir указывает исходный каталог медиафайлов текущей коллекции Anki.
	MediaDir string `json:"media_dir"`
	// IncludeMedia определяет, нужно ли копировать медиафайлы.
	IncludeMedia bool `json:"include_media"`
	// RootDeckIDs содержит идентификаторы корневых экспортируемых колод.
	RootDeckIDs []string `json:"root_deck_ids"`
	// Decks содержит список экспортируемых колод.
	Decks []DeckDTO `json:"decks"`
	// NoteTypes содержит спецификации типов заметок, используемых в экспорте.
	NoteTypes []NoteTypeDTO `json:"note_types"`
	// Notes содержит экспортируемые заметки.
	Notes []NoteDTO `json:"notes"`
	// Cards содержит экспортируемые карточки.
	Cards []CardDTO `json:"cards"`
	// MediaFiles содержит дедуплицированный список относительных имен медиафайлов.
	MediaFiles []string `json:"media_files"`
}

// DeckDTO представляет колоду в транспортном протоколе и файле decks.json.
type DeckDTO struct {
	ID          string  `json:"id"`
	ParentID    *string `json:"parent_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
}

// NoteFieldDTO описывает описание поля типа заметки.
type NoteFieldDTO struct {
	Name string `json:"name"`
	Ord  int    `json:"ord"`
}

// CardTemplateDTO описывает шаблон карточки в типе заметки.
type CardTemplateDTO struct {
	Name  string `json:"name"`
	Ord   int    `json:"ord"`
	Qfmt  string `json:"qfmt"`
	Afmt  string `json:"afmt"`
	Bqfmt string `json:"bqfmt,omitempty"`
	Bafmt string `json:"bafmt,omitempty"`
}

// NoteTypeDTO представляет тип заметки в транспортном протоколе и каталоге note_types/.
type NoteTypeDTO struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Fields    []NoteFieldDTO    `json:"fields"`
	Templates []CardTemplateDTO `json:"templates"`
	CSS       string            `json:"css"`
}

// NoteDTO представляет заметку в транспортном протоколе и строке notes.jsonl.
type NoteDTO struct {
	GUID       string            `json:"guid"`
	NoteTypeID string            `json:"note_type_id"`
	NoteType   string            `json:"note_type"`
	Fields     map[string]string `json:"fields"`
	Tags       []string          `json:"tags"`
}

// CardDTO представляет карточку в транспортном протоколе и строке cards.jsonl.
type CardDTO struct {
	NoteGUID string `json:"note_guid"`
	CardID   string `json:"card_id"`
	Ord      int    `json:"ord"`
	DeckID   string `json:"deck_id"`
}

// ManifestMeta представляет файл-маркер crowdanki.json.
type ManifestMeta struct {
	Format        string   `json:"format"`
	SchemaVersion int      `json:"schema_version"`
	RootDecks     []string `json:"root_decks"`
}

// MediaItem описывает один успешно экспортированный медиафайл.
type MediaItem struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// MediaManifest описывает файл media.json.
type MediaManifest struct {
	Included bool        `json:"included"`
	Files    []MediaItem `json:"files"`
	Missing  []string    `json:"missing"`
}

// ExportResult возвращается Python-адаптеру со структурированными итогами экспорта.
type ExportResult struct {
	Decks          int   `json:"decks"`
	Notes          int   `json:"notes"`
	Cards          int   `json:"cards"`
	NoteTypes      int   `json:"note_types"`
	MediaFiles     int   `json:"media_files"`
	MediaBytes     int64 `json:"media_bytes"`
	MissingMedia   int   `json:"missing_media"`
	ElapsedMs      int64 `json:"elapsed_ms"`
	MediaElapsedMs int64 `json:"media_elapsed_ms"`
}
