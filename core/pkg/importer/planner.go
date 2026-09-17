package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AliceLiddell01/CrowdAnki_V2/core/pkg/export"
)

// PlanImport выполняет чтение, валидацию и построение детерминированного плана импорта.
func PlanImport(req PlanImportRequest) (*ImportPlanResult, error) {
	src, valErrs, err := ReadAndValidateSourceProject(req.SourceDir)
	if err != nil {
		// Если обнаружены структурированные ошибки валидации, возвращаем план с конфликтами валидации
		if len(valErrs) > 0 {
			conflicts := make([]ConflictItem, len(valErrs))
			for i, v := range valErrs {
				conflicts[i] = ConflictItem{
					Category: "validation",
					Entity:   "project",
					Message:  v,
				}
			}
			return &ImportPlanResult{
				CanApply:    false,
				RootDecks:   make([]string, 0),
				Summary:     PlanSummary{TotalConflicts: len(conflicts)},
				Conflicts:   conflicts,
				Warnings:    make([]string, 0),
				DeckOps:     make([]DeckOp, 0),
				NoteTypeOps: make([]NoteTypeOp, 0),
				NoteOps:     make([]NoteOp, 0),
				CardOps:     make([]CardOp, 0),
				MediaOps:    make([]MediaOp, 0),
			}, nil
		}
		return nil, err
	}

	result := &ImportPlanResult{
		CanApply:    true,
		RootDecks:   src.Manifest.RootDecks,
		Conflicts:   make([]ConflictItem, 0),
		Warnings:    make([]string, 0),
		DeckOps:     make([]DeckOp, 0),
		NoteTypeOps: make([]NoteTypeOp, 0),
		NoteOps:     make([]NoteOp, 0),
		CardOps:     make([]CardOp, 0),
		MediaOps:    make([]MediaOp, 0),
	}

	// 1. Определение import scope
	isDeckInScope := func(deckName string) bool {
		for _, root := range src.Manifest.RootDecks {
			if deckName == root || strings.HasPrefix(deckName, root+"::") {
				return true
			}
		}
		return false
	}

	// 2. Синхронизация колод
	destDeckByName := make(map[string]DestDeckDTO, len(req.DestSnapshot.Decks))
	for _, d := range req.DestSnapshot.Decks {
		destDeckByName[d.Name] = d
	}

	for _, sDeck := range src.Decks {
		if dDeck, exists := destDeckByName[sDeck.Name]; exists {
			result.DeckOps = append(result.DeckOps, DeckOp{
				Action:      "no-op",
				DeckID:      dDeck.ID,
				Name:        sDeck.Name,
				Description: sDeck.Description,
			})
		} else {
			result.DeckOps = append(result.DeckOps, DeckOp{
				Action:      "create",
				DeckID:      sDeck.ID,
				Name:        sDeck.Name,
				Description: sDeck.Description,
			})
			result.Summary.CreatedDecks++
		}
	}

	// Поиск колод на удаление внутри scope
	for _, dDeck := range req.DestSnapshot.Decks {
		if isDeckInScope(dDeck.Name) {
			if _, exists := src.DeckByName[dDeck.Name]; !exists {
				result.DeckOps = append(result.DeckOps, DeckOp{
					Action:      "delete",
					DeckID:      dDeck.ID,
					Name:        dDeck.Name,
					Description: dDeck.Description,
				})
				result.Summary.DeletedDecks++
			}
		}
	}

	// 3. Синхронизация типов заметок и выявление конфликтов
	destNTByName := make(map[string]DestNoteTypeDTO, len(req.DestSnapshot.NoteTypes))
	for _, nt := range req.DestSnapshot.NoteTypes {
		destNTByName[nt.Name] = nt
	}

	for _, sNT := range src.NoteTypes {
		dNT, exists := destNTByName[sNT.Name]
		if !exists {
			result.NoteTypeOps = append(result.NoteTypeOps, NoteTypeOp{
				Action:    "create",
				SourceID:  sNT.ID,
				Name:      sNT.Name,
				Kind:      sNT.Kind,
				Sortf:     sNT.Sortf,
				Fields:    convertSourceFields(sNT.Fields),
				Templates: convertSourceTemplates(sNT.Templates),
				CSS:       sNT.CSS,
			})
			result.Summary.CreatedNoteTypes++
			continue
		}

		// Сравнение полей и шаблонов
		fieldDiff, fieldsEqual := diffFields(sNT.Fields, dNT.Fields)
		tmplDiff, tmplsEqual := diffTemplates(sNT.Templates, dNT.Templates)
		cssEqual := (sNT.CSS == dNT.CSS)

		if fieldsEqual && tmplsEqual && cssEqual {
			result.NoteTypeOps = append(result.NoteTypeOps, NoteTypeOp{
				Action:    "no-op",
				SourceID:  sNT.ID,
				DestID:    dNT.ID,
				Name:      sNT.Name,
				Kind:      sNT.Kind,
				Sortf:     sNT.Sortf,
				Fields:    dNT.Fields,
				Templates: dNT.Templates,
				CSS:       dNT.CSS,
			})
			continue
		}

		// Проверка на конфликт полей (неоднозначные изменения)
		if fieldDiff.HasConflict {
			conflictMsg := fmt.Sprintf("Неоднозначное изменение типа заметки «%s»: %s", sNT.Name, fieldDiff.ConflictReason)
			result.Conflicts = append(result.Conflicts, ConflictItem{
				Category: "notetype",
				Entity:   sNT.Name,
				Message:  conflictMsg,
			})
			result.NoteTypeOps = append(result.NoteTypeOps, NoteTypeOp{
				Action:   "conflict",
				SourceID: sNT.ID,
				DestID:   dNT.ID,
				Name:     sNT.Name,
				Details:  conflictMsg,
			})
			result.Summary.TotalConflicts++
			continue
		}

		if tmplDiff.HasConflict {
			conflictMsg := fmt.Sprintf("Неоднозначное изменение шаблонов типа заметки «%s»: %s", sNT.Name, tmplDiff.ConflictReason)
			result.Conflicts = append(result.Conflicts, ConflictItem{
				Category: "notetype",
				Entity:   sNT.Name,
				Message:  conflictMsg,
			})
			result.NoteTypeOps = append(result.NoteTypeOps, NoteTypeOp{
				Action:   "conflict",
				SourceID: sNT.ID,
				DestID:   dNT.ID,
				Name:     sNT.Name,
				Details:  conflictMsg,
			})
			result.Summary.TotalConflicts++
			continue
		}

		// Однозначное обновление
		var details []string
		if !fieldsEqual {
			details = append(details, "добавлены новые поля")
		}
		if !tmplsEqual {
			details = append(details, "обновлены шаблоны карточек")
		}
		if !cssEqual {
			details = append(details, "обновлены стили CSS")
		}

		result.NoteTypeOps = append(result.NoteTypeOps, NoteTypeOp{
			Action:    "update",
			SourceID:  sNT.ID,
			DestID:    dNT.ID,
			Name:      sNT.Name,
			Kind:      sNT.Kind,
			Sortf:     sNT.Sortf,
			Fields:    convertSourceFields(sNT.Fields),
			Templates: convertSourceTemplates(sNT.Templates),
			CSS:       sNT.CSS,
			Details:   strings.Join(details, ", "),
		})
		result.Summary.UpdatedNoteTypes++
	}

	// 4. Синхронизация заметок и карточек
	destNoteByGUID := make(map[string]DestNoteDTO, len(req.DestSnapshot.Notes))
	for _, n := range req.DestSnapshot.Notes {
		destNoteByGUID[n.GUID] = n
	}

	// Индексация карточек назначения: (note_guid, ord) -> DestCardDTO
	destCardByLogicalKey := make(map[string]DestCardDTO, len(req.DestSnapshot.Cards))
	// Карточки по заметкам
	destCardsByNoteGUID := make(map[string][]DestCardDTO)
	for _, c := range req.DestSnapshot.Cards {
		key := fmt.Sprintf("%s:%d", c.NoteGUID, c.Ord)
		destCardByLogicalKey[key] = c
		destCardsByNoteGUID[c.NoteGUID] = append(destCardsByNoteGUID[c.NoteGUID], c)
	}

	// Source cards by note GUID
	srcCardsByNoteGUID := make(map[string][]export.CardDTO)
	for _, c := range src.Cards {
		srcCardsByNoteGUID[c.NoteGUID] = append(srcCardsByNoteGUID[c.NoteGUID], c)
	}

	// Обработка заметок из проекта
	for _, sNote := range src.Notes {
		firstField := extractFirstFieldValue(sNote.Fields)
		dNote, exists := destNoteByGUID[sNote.GUID]
		if !exists {
			result.NoteOps = append(result.NoteOps, NoteOp{
				Action:       "create",
				GUID:         sNote.GUID,
				NoteTypeName: sNote.NoteType,
				Fields:       sNote.Fields,
				Tags:         sNote.Tags,
				FirstField:   firstField,
			})
			result.Summary.CreatedNotes++
		} else {
			// Сравнение полей и тегов
			fieldsChanged := !areFieldsEqual(sNote.Fields, dNote.Fields)
			tagsChanged := !areTagsEqual(sNote.Tags, dNote.Tags)
			typeChanged := (sNote.NoteType != dNote.NoteType)

			if fieldsChanged || tagsChanged || typeChanged {
				result.NoteOps = append(result.NoteOps, NoteOp{
					Action:       "update",
					GUID:         sNote.GUID,
					DestID:       dNote.ID,
					NoteTypeName: sNote.NoteType,
					Fields:       sNote.Fields,
					Tags:         sNote.Tags,
					FirstField:   firstField,
				})
				result.Summary.UpdatedNotes++
			} else {
				result.NoteOps = append(result.NoteOps, NoteOp{
					Action:       "no-op",
					GUID:         sNote.GUID,
					DestID:       dNote.ID,
					NoteTypeName: sNote.NoteType,
					Fields:       sNote.Fields,
					Tags:         sNote.Tags,
					FirstField:   firstField,
				})
			}
		}

		// Обработка карточек для заметки
		for _, sCard := range srcCardsByNoteGUID[sNote.GUID] {
			targetDeck := src.DeckByID[sCard.DeckID].Name
			cardKey := fmt.Sprintf("%s:%d", sCard.NoteGUID, sCard.Ord)
			dCard, cardExists := destCardByLogicalKey[cardKey]

			if !cardExists {
				result.CardOps = append(result.CardOps, CardOp{
					Action:      "create",
					NoteGUID:    sCard.NoteGUID,
					Ord:         sCard.Ord,
					NewDeck:     targetDeck,
					NotePreview: firstField,
				})
				result.Summary.CreatedCards++
			} else {
				if dCard.DeckName != targetDeck {
					result.CardOps = append(result.CardOps, CardOp{
						Action:      "move",
						DestID:      dCard.ID,
						NoteGUID:    sCard.NoteGUID,
						Ord:         sCard.Ord,
						OldDeck:     dCard.DeckName,
						NewDeck:     targetDeck,
						NotePreview: firstField,
					})
					result.Summary.MovedCards++
				} else {
					result.CardOps = append(result.CardOps, CardOp{
						Action:      "no-op",
						DestID:      dCard.ID,
						NoteGUID:    sCard.NoteGUID,
						Ord:         sCard.Ord,
						OldDeck:     dCard.DeckName,
						NewDeck:     targetDeck,
						NotePreview: firstField,
					})
				}
			}
		}
	}

	// Обработка удалений карточек и заметок назначения
	for _, dNote := range req.DestSnapshot.Notes {
		dCards := destCardsByNoteGUID[dNote.GUID]

		// Имеет ли заметка карточки в scope
		var inScopeCards []DestCardDTO
		var outOfScopeCards []DestCardDTO
		for _, dc := range dCards {
			if dc.InScope || isDeckInScope(dc.DeckName) {
				inScopeCards = append(inScopeCards, dc)
			} else {
				outOfScopeCards = append(outOfScopeCards, dc)
			}
		}

		if len(inScopeCards) == 0 {
			// Заметка полностью вне scope — игнорируем
			continue
		}

		_, inSource := src.NoteByGUID[dNote.GUID]
		if !inSource {
			// Заметка отсутствует в источнике!
			// 1. Все её карточки в scope планируются на удаление
			for _, sc := range inScopeCards {
				result.CardOps = append(result.CardOps, CardOp{
					Action:      "delete",
					DestID:      sc.ID,
					NoteGUID:    sc.NoteGUID,
					Ord:         sc.Ord,
					OldDeck:     sc.DeckName,
					NotePreview: extractFirstFieldValue(dNote.Fields),
				})
				result.Summary.DeletedCards++
			}

			// 2. Проверка защиты карточек вне scope!
			if len(outOfScopeCards) > 0 {
				// Заметка НЕ удаляется целиком, так как карточки вне scope сохраняются!
				// result.NoteOps НЕ содержит delete для dNote!
			} else {
				// Все карточки заметки были в scope и будут удалены -> безопасное удаление Note
				result.NoteOps = append(result.NoteOps, NoteOp{
					Action:       "delete",
					GUID:         dNote.GUID,
					DestID:       dNote.ID,
					NoteTypeName: dNote.NoteType,
					Fields:       dNote.Fields,
					Tags:         dNote.Tags,
					FirstField:   extractFirstFieldValue(dNote.Fields),
				})
				result.Summary.DeletedNotes++
			}
		} else {
			// Заметка есть в источнике, но некоторые её карточки в scope могут отсутствовать в источнике
			for _, sc := range inScopeCards {
				cardKey := fmt.Sprintf("%s:%d", sc.NoteGUID, sc.Ord)
				// Проверяем, есть ли эта карточка в источнике
				sourceHasCard := false
				for _, scSrc := range srcCardsByNoteGUID[dNote.GUID] {
					if scSrc.Ord == sc.Ord {
						sourceHasCard = true
						break
					}
				}
				if !sourceHasCard {
					result.CardOps = append(result.CardOps, CardOp{
						Action:      "delete",
						DestID:      sc.ID,
						NoteGUID:    sc.NoteGUID,
						Ord:         sc.Ord,
						OldDeck:     sc.DeckName,
						NotePreview: extractFirstFieldValue(dNote.Fields),
					})
					result.Summary.DeletedCards++
					_ = cardKey
				}
			}
		}
	}

	// 5. Синхронизация медиафайлов
	if src.Media != nil && len(src.Media.Files) > 0 {
		for _, mf := range src.Media.Files {
			srcFilePath := filepath.Join(src.MediaDir, mf.Name)
			srcFileStat, statErr := os.Stat(srcFilePath)
			if statErr != nil || srcFileStat.IsDir() {
				// Файл заявлен в манифесте, но отсутствует на диске
				result.MediaOps = append(result.MediaOps, MediaOp{
					Action:       "missing",
					Name:         mf.Name,
					SourcePath:   srcFilePath,
					Size:         mf.Size,
					SourceSHA256: mf.SHA256,
				})
				result.Summary.MissingMedia++
				if req.IncludeMedia {
					if req.MediaConflictStrategy == "skip" {
						result.Warnings = append(result.Warnings, fmt.Sprintf("Медиафайл «%s» указан в media.json, но физически отсутствует в каталоге media/ проекта (пропущен)", mf.Name))
					} else {
						result.Conflicts = append(result.Conflicts, ConflictItem{
							Category: "media",
							Entity:   mf.Name,
							Message:  fmt.Sprintf("Медиафайл «%s» указан в media.json, но физически отсутствует в каталоге media/ проекта", mf.Name),
						})
						result.Summary.TotalConflicts++
					}
				}
				continue
			}

			// Проверка в каталоге Anki media
			destFilePath := filepath.Join(req.MediaDir, mf.Name)
			destStat, destErr := os.Stat(destFilePath)
			if destErr != nil {
				// Файла нет в Anki -> добавляем
				result.MediaOps = append(result.MediaOps, MediaOp{
					Action:       "add",
					Name:         mf.Name,
					SourcePath:   srcFilePath,
					Size:         mf.Size,
					SourceSHA256: mf.SHA256,
				})
				result.Summary.AddedMedia++
			} else if !destStat.IsDir() {
				// Файл существует -> проверяем SHA-256
				destHash, hashErr := calculateFileSHA256(destFilePath)
				if hashErr == nil && strings.EqualFold(destHash, mf.SHA256) {
					result.MediaOps = append(result.MediaOps, MediaOp{
						Action:       "same",
						Name:         mf.Name,
						SourcePath:   srcFilePath,
						Size:         mf.Size,
						SourceSHA256: mf.SHA256,
						DestSHA256:   destHash,
					})
					result.Summary.SameMedia++
				} else {
					action := "conflict"
					if req.IncludeMedia {
						switch req.MediaConflictStrategy {
						case "skip":
							action = "skip"
							result.Warnings = append(result.Warnings, fmt.Sprintf("Конфликт медиафайла «%s»: хэш в коллекции отличается от проекта (%s vs %s) — сохранён существующий файл коллекции", mf.Name, destHash, mf.SHA256))
						case "overwrite":
							action = "overwrite"
							result.Warnings = append(result.Warnings, fmt.Sprintf("Конфликт медиафайла «%s»: хэш в коллекции отличается от проекта (%s vs %s) — файл будет перезаписан", mf.Name, destHash, mf.SHA256))
						default: // "block" или пусто
							action = "conflict"
							result.Conflicts = append(result.Conflicts, ConflictItem{
								Category: "media",
								Entity:   mf.Name,
								Message:  fmt.Sprintf("Конфликт медиафайла «%s»: хэш SHA-256 в коллекции Anki отличается от файла в проекте (%s vs %s)", mf.Name, destHash, mf.SHA256),
							})
							result.Summary.TotalConflicts++
						}
					}
					result.MediaOps = append(result.MediaOps, MediaOp{
						Action:       action,
						Name:         mf.Name,
						SourcePath:   srcFilePath,
						Size:         mf.Size,
						SourceSHA256: mf.SHA256,
						DestSHA256:   destHash,
					})
					result.Summary.ConflictMedia++
				}
			}
		}
	}

	// 6. Подсчет общей сводки проекта
	result.Summary.TotalDecks = len(src.Decks)
	result.Summary.TotalNotes = len(src.Notes)
	result.Summary.TotalCards = len(src.Cards)
	result.Summary.TotalNoteTypes = len(src.NoteTypes)
	if src.Media != nil {
		result.Summary.TotalMediaFiles = len(src.Media.Files)
		var totalBytes int64
		for _, f := range src.Media.Files {
			totalBytes += f.Size
		}
		result.Summary.TotalMediaBytes = totalBytes
	}

	// 7. Проверка блокирующих факторов (CanApply)
	if len(result.Conflicts) > 0 {
		result.CanApply = false
	}

	// 8. Формирование предупреждений о деструктивных операциях
	var destrParts []string
	if result.Summary.DeletedNotes > 0 {
		destrParts = append(destrParts, fmt.Sprintf("%d заметок", result.Summary.DeletedNotes))
	}
	if result.Summary.DeletedCards > 0 {
		destrParts = append(destrParts, fmt.Sprintf("%d карточек", result.Summary.DeletedCards))
	}
	if result.Summary.DeletedDecks > 0 {
		destrParts = append(destrParts, fmt.Sprintf("%d колод", result.Summary.DeletedDecks))
	}
	if len(destrParts) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Будут удалены %s.", strings.Join(destrParts, ", ")))
	}

	// 9. Детерминированная сортировка всех операций в плане
	sort.Slice(result.DeckOps, func(i, j int) bool {
		if result.DeckOps[i].Action != result.DeckOps[j].Action {
			return result.DeckOps[i].Action < result.DeckOps[j].Action
		}
		return result.DeckOps[i].Name < result.DeckOps[j].Name
	})

	sort.Slice(result.NoteTypeOps, func(i, j int) bool {
		return result.NoteTypeOps[i].Name < result.NoteTypeOps[j].Name
	})

	sort.Slice(result.NoteOps, func(i, j int) bool {
		return result.NoteOps[i].GUID < result.NoteOps[j].GUID
	})

	sort.Slice(result.CardOps, func(i, j int) bool {
		if result.CardOps[i].NoteGUID != result.CardOps[j].NoteGUID {
			return result.CardOps[i].NoteGUID < result.CardOps[j].NoteGUID
		}
		return result.CardOps[i].Ord < result.CardOps[j].Ord
	})

	sort.Slice(result.MediaOps, func(i, j int) bool {
		return result.MediaOps[i].Name < result.MediaOps[j].Name
	})

	sort.Slice(result.Conflicts, func(i, j int) bool {
		if result.Conflicts[i].Category != result.Conflicts[j].Category {
			return result.Conflicts[i].Category < result.Conflicts[j].Category
		}
		return result.Conflicts[i].Entity < result.Conflicts[j].Entity
	})

	return result, nil
}

type fieldDiffResult struct {
	HasConflict    bool
	ConflictReason string
}

func diffFields(src []export.NoteFieldDTO, dst []DestFieldDTO) (fieldDiffResult, bool) {
	if len(src) == len(dst) {
		equal := true
		for i := range src {
			if src[i].Name != dst[i].Name {
				equal = false
				break
			}
		}
		if equal {
			return fieldDiffResult{}, true
		}
		// Одинаковое число полей, но разные имена -> неоднозначно (переименование/замена)
		return fieldDiffResult{
			HasConflict:    true,
			ConflictReason: "изменены имена существующих полей",
		}, false
	}

	if len(src) < len(dst) {
		// В источнике полей меньше, чем в коллекции -> удаление полей неоднозначно
		return fieldDiffResult{
			HasConflict:    true,
			ConflictReason: fmt.Sprintf("в проекте меньше полей (%d), чем в Anki (%d)", len(src), len(dst)),
		}, false
	}

	// len(src) > len(dst): проверяем, что все поля dst сохранены в том же порядке в начале
	for i := range dst {
		if src[i].Name != dst[i].Name {
			return fieldDiffResult{
				HasConflict:    true,
				ConflictReason: fmt.Sprintf("поле '%s' не совпадает с существующим полем '%s'", src[i].Name, dst[i].Name),
			}, false
		}
	}

	// Однозначное добавление новых полей в конец
	return fieldDiffResult{HasConflict: false}, false
}

type tmplDiffResult struct {
	HasConflict    bool
	ConflictReason string
}

func diffTemplates(src []export.CardTemplateDTO, dst []DestTemplateDTO) (tmplDiffResult, bool) {
	if len(src) < len(dst) {
		return tmplDiffResult{
			HasConflict:    true,
			ConflictReason: fmt.Sprintf("в проекте меньше шаблонов (%d), чем в Anki (%d)", len(src), len(dst)),
		}, false
	}

	if len(src) == len(dst) {
		equal := true
		for i := range src {
			if src[i].Name != dst[i].Name ||
				src[i].Qfmt != dst[i].Qfmt ||
				src[i].Afmt != dst[i].Afmt ||
				src[i].Bqfmt != dst[i].Bqfmt ||
				src[i].Bafmt != dst[i].Bafmt {
				equal = false
				break
			}
		}
		if equal {
			return tmplDiffResult{}, true
		}
		// Шаблоны модифицированы (qfmt/afmt/css) — это безопасное обновление
		return tmplDiffResult{HasConflict: false}, false
	}

	// len(src) > len(dst)
	for i := range dst {
		if src[i].Name != dst[i].Name {
			return tmplDiffResult{
				HasConflict:    true,
				ConflictReason: fmt.Sprintf("шаблон '%s' не совпадает с существующим шаблоном '%s'", src[i].Name, dst[i].Name),
			}, false
		}
	}

	// Новые шаблоны в конце
	return tmplDiffResult{HasConflict: false}, false
}

func convertSourceFields(fields []export.NoteFieldDTO) []DestFieldDTO {
	res := make([]DestFieldDTO, len(fields))
	for i, f := range fields {
		res[i] = DestFieldDTO{Name: f.Name, Ord: f.Ord}
	}
	return res
}

func convertSourceTemplates(tmpls []export.CardTemplateDTO) []DestTemplateDTO {
	res := make([]DestTemplateDTO, len(tmpls))
	for i, t := range tmpls {
		res[i] = DestTemplateDTO{
			Name:  t.Name,
			Ord:   t.Ord,
			Qfmt:  t.Qfmt,
			Afmt:  t.Afmt,
			Bqfmt: t.Bqfmt,
			Bafmt: t.Bafmt,
		}
	}
	return res
}

func areFieldsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func areTagsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sa := make([]string, len(a))
	copy(sa, a)
	sort.Strings(sa)

	sb := make([]string, len(b))
	copy(sb, b)
	sort.Strings(sb)

	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

func extractFirstFieldValue(fields map[string]string) string {
	if len(fields) == 0 {
		return ""
	}
	// Если есть стандартные имена первого поля
	for _, preferredKey := range []string{"Front", "Лицо", "Слово", "Вопрос", "Text", "Field 1"} {
		if val, ok := fields[preferredKey]; ok && strings.TrimSpace(val) != "" {
			return trimForPreview(val)
		}
	}
	// Первое непустое поле по алфавиту ключей
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := fields[k]
		if strings.TrimSpace(val) != "" {
			return trimForPreview(val)
		}
	}
	return ""
}

func trimForPreview(text string) string {
	cleaned := strings.ReplaceAll(text, "\n", " ")
	cleaned = strings.TrimSpace(cleaned)
	runes := []rune(cleaned)
	if len(runes) > 60 {
		return string(runes[:57]) + "…"
	}
	return cleaned
}

func calculateFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
