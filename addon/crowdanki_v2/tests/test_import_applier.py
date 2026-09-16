"""Тесты применения ImportPlan к коллекции Anki."""

import unittest

from crowdanki_v2.import_applier import ImportApplicationError, apply_import_plan


class FakeDeckManager:
    def __init__(self):
        self.decks = {
            1: {"id": 1, "name": "Default"},
            10: {"id": 10, "name": "文法"},
            20: {"id": 20, "name": "文法::N2"},
            30: {"id": 30, "name": "文法::СтарыйУрок"},
        }
        self.next_id = 100
        self.deleted_deck_ids = []

    def id(self, name: str, create: bool = True):
        for did, d in self.decks.items():
            if d["name"] == name:
                return did
        if create:
            self.next_id += 1
            self.decks[self.next_id] = {"id": self.next_id, "name": name}
            return self.next_id
        return None

    def remove(self, dids: list[int]):
        self.deleted_deck_ids.extend(dids)
        for did in dids:
            self.decks.pop(did, None)


class FakeCard:
    def __init__(self, cid: int, nid: int, did: int, ord_num: int):
        self.id = cid
        self.nid = nid
        self.did = did
        self.ord = ord_num
        # Атрибуты обучения
        self.reps = 15
        self.interval = 30
        self.ease = 2500
        self.due = 12345


class FakeNote:
    def __init__(self, nid: int, guid: str, mid: int, fields: dict, tags: list):
        self.id = nid
        self.guid = guid
        self.mid = mid
        self._fields = dict(fields)
        self.tags = list(tags)

    def __contains__(self, key):
        return key in self._fields

    def __getitem__(self, key):
        return self._fields[key]

    def __setitem__(self, key, val):
        self._fields[key] = val


class FakeModelManager:
    def __init__(self):
        self.models = {
            100: {
                "id": 100,
                "name": "Базовый",
                "type": 0,
                "sortf": 0,
                "css": ".card { font-size: 14px; }",
                "flds": [{"name": "Front", "ord": 0}, {"name": "Back", "ord": 1}],
                "tmpls": [{"name": "Card 1", "ord": 0, "qfmt": "{{Front}}", "afmt": "{{Back}}"}],
            }
        }
        self.next_id = 500

    def by_name(self, name: str):
        for m in self.models.values():
            if m["name"] == name:
                return m
        return None

    def new(self, name: str):
        self.next_id += 1
        return {
            "id": self.next_id,
            "name": name,
            "type": 0,
            "sortf": 0,
            "css": "",
            "flds": [],
            "tmpls": [],
        }

    def new_field(self, name: str):
        return {"name": name, "ord": 0}

    def add_field(self, model: dict, field: dict):
        field["ord"] = len(model["flds"])
        model["flds"].append(field)

    def new_template(self, name: str):
        return {"name": name, "ord": 0, "qfmt": "", "afmt": ""}

    def add_template(self, model: dict, tmpl: dict):
        tmpl["ord"] = len(model["tmpls"])
        model["tmpls"].append(tmpl)

    def save(self, model: dict):
        self.models[model["id"]] = model


class FakeMediaManager:
    def __init__(self):
        self.added_files = []

    def add_file(self, path: str) -> str:
        self.added_files.append(path)
        return path


class FakeCollection:
    def __init__(self):
        self.decks = FakeDeckManager()
        self.models = FakeModelManager()
        self.media = FakeMediaManager()
        self.notes: dict[int, FakeNote] = {
            1001: FakeNote(
                1001, "guid-existing", 100, {"Front": "Старое", "Back": "Старый"}, ["tag1"]
            ),
            1002: FakeNote(1002, "guid-delete", 100, {"Front": "Удаляемая", "Back": "Удалить"}, []),
        }
        self.cards: dict[int, FakeCard] = {
            2001: FakeCard(2001, 1001, 10, 0),  # будет перемещена в 20
            2002: FakeCard(2002, 1002, 30, 0),  # будет удалена
        }
        self.next_nid = 2000
        self.deleted_cards = []
        self.deleted_notes = []
        self.saved = False

    def get_note(self, nid: int):
        return self.notes.get(nid)

    def find_notes(self, query: str):
        guid = query.replace("guid:", "")
        return [nid for nid, n in self.notes.items() if n.guid == guid]

    def new_note(self, model: dict):
        self.next_nid += 1
        fields = {f["name"]: "" for f in model["flds"]}
        return FakeNote(self.next_nid, "", model["id"], fields, [])

    def add_note(self, note: FakeNote, deck_id: int):
        self.notes[note.id] = note

    def update_note(self, note: FakeNote):
        self.notes[note.id] = note

    def set_deck(self, card_ids: list[int], deck_id: int):
        for cid in card_ids:
            if cid in self.cards:
                self.cards[cid].did = deck_id

    def remove_cards_and_orphaned_notes(self, card_ids: list[int]):
        self.deleted_cards.extend(card_ids)
        for cid in card_ids:
            self.cards.pop(cid, None)

    def remove_notes(self, note_ids: list[int]):
        self.deleted_notes.extend(note_ids)
        for nid in note_ids:
            self.notes.pop(nid, None)

    def save(self):
        self.saved = True


class TestImportApplier(unittest.TestCase):
    def test_apply_import_plan_cannot_apply_conflict(self):
        col = FakeCollection()
        bad_plan = {"can_apply": False}
        with self.assertRaises(ImportApplicationError):
            apply_import_plan(col, bad_plan, include_media=False)

    def test_apply_import_plan_comprehensive(self):
        col = FakeCollection()

        plan = {
            "can_apply": True,
            "summary": {
                "created_decks": 1,
                "deleted_decks": 1,
                "created_notes": 1,
                "updated_notes": 1,
                "deleted_notes": 1,
                "created_cards": 1,
                "moved_cards": 1,
                "deleted_cards": 1,
                "added_media": 1,
            },
            "deck_ops": [
                {"action": "create", "name": "文法::N2::НовыйУрок"},
                {"action": "delete", "deck_id": "30", "name": "文法::СтарыйУрок"},
            ],
            "note_type_ops": [
                {
                    "action": "create",
                    "name": "НовыйТип",
                    "kind": "standard",
                    "sortf": 0,
                    "fields": [{"name": "Слово", "ord": 0}],
                    "templates": [
                        {"name": "Карточка 1", "ord": 0, "qfmt": "{{Слово}}", "afmt": "{{Слово}}"}
                    ],
                }
            ],
            "note_ops": [
                {
                    "action": "create",
                    "guid": "guid-new",
                    "note_type_name": "Базовый",
                    "fields": {"Front": "Новое слово", "Back": "Новый перевод"},
                    "tags": ["new-tag"],
                },
                {
                    "action": "update",
                    "guid": "guid-existing",
                    "dest_id": "1001",
                    "note_type_name": "Базовый",
                    "fields": {"Front": "Обновленное", "Back": "Обновленный"},
                    "tags": ["updated-tag"],
                },
                {
                    "action": "delete",
                    "dest_id": "1002",
                    "guid": "guid-delete",
                },
            ],
            "card_ops": [
                {
                    "action": "create",
                    "note_guid": "guid-new",
                    "ord": 0,
                    "new_deck": "文法::N2::НовыйУрок",
                },
                {
                    "action": "move",
                    "dest_id": "2001",
                    "note_guid": "guid-existing",
                    "ord": 0,
                    "old_deck": "文法",
                    "new_deck": "文法::N2",
                },
                {
                    "action": "delete",
                    "dest_id": "2002",
                    "note_guid": "guid-delete",
                    "ord": 0,
                },
            ],
            "media_ops": [
                {
                    "action": "add",
                    "name": "test.mp3",
                    "source_path": "/fake/test.mp3",
                }
            ],
        }

        # Сохраняем исходное состояние карточки 2001 для проверки инварианта сохранения scheduling
        card_2001_before = col.cards[2001]
        orig_reps = card_2001_before.reps
        orig_interval = card_2001_before.interval
        orig_ease = card_2001_before.ease
        orig_due = card_2001_before.due

        res = apply_import_plan(col, plan, include_media=True)

        # 1. Проверяем создание нового типа заметок
        self.assertIsNotNone(col.models.by_name("НовыйТип"))

        # 2. Проверяем создание колоды
        self.assertIsNotNone(col.decks.id("文法::N2::НовыйУрок", create=False))

        # 3. Проверяем удаление колоды 30
        self.assertIn(30, col.decks.deleted_deck_ids)

        # 4. Проверяем создание новой заметки
        new_note_ids = col.find_notes("guid:guid-new")
        self.assertEqual(len(new_note_ids), 1)
        new_note = col.get_note(new_note_ids[0])
        self.assertEqual(new_note["Front"], "Новое слово")
        self.assertIn("new-tag", new_note.tags)

        # 5. Проверяем обновление существующей заметки
        updated_note = col.get_note(1001)
        self.assertEqual(updated_note["Front"], "Обновленное")
        self.assertIn("updated-tag", updated_note.tags)

        # 6. Проверяем перемещение карточки 2001 в 文法::N2 (ID 20)
        card_2001_after = col.cards[2001]
        self.assertEqual(card_2001_after.did, 20)

        # 7. Инвариант: прогресс обучения существующей карточки 2001 СОХРАНЕН!
        self.assertEqual(card_2001_after.reps, orig_reps)
        self.assertEqual(card_2001_after.interval, orig_interval)
        self.assertEqual(card_2001_after.ease, orig_ease)
        self.assertEqual(card_2001_after.due, orig_due)

        # 8. Проверяем удаление карточки 2002 и заметки 1002
        self.assertIn(2002, col.deleted_cards)
        self.assertIn(1002, col.deleted_notes)

        # 9. Проверяем добавление медиа
        self.assertIn("/fake/test.mp3", col.media.added_files)

        # 10. Проверяем сохранение коллекции
        self.assertTrue(col.saved)

        # 11. Проверяем итог
        self.assertEqual(res["created_notes"], 1)
        self.assertEqual(res["updated_notes"], 1)
        self.assertEqual(res["deleted_notes"], 1)
        self.assertEqual(res["moved_cards"], 1)
        self.assertEqual(res["added_media"], 1)


if __name__ == "__main__":
    unittest.main()
