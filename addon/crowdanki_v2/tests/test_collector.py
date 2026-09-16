"""Тесты сбора данных коллекции для экспорта."""

import unittest

from crowdanki_v2.collector import collect_export_data, get_deck_and_child_ids


class FakeDeckManager:
    def __init__(self):
        self.decks = {
            1: {"id": 1, "name": "Default", "desc": ""},
            10: {"id": 10, "name": "Японский", "desc": "Корень"},
            20: {"id": 20, "name": "Японский::N5", "desc": "Подколода"},
            30: {"id": 30, "name": "Японский::N5::Кандзи", "desc": "Подподколода"},
        }

    def get(self, did):
        return self.decks.get(did)

    def by_name(self, name):
        for d in self.decks.values():
            if d["name"] == name:
                return d
        return None

    def children(self, root_did):
        """Имитирует РЕАЛЬНЫЙ контракт Anki 26.09.2: список кортежей (name, DeckId)."""
        if root_did == 10:
            return [("Японский::N5", 20), ("Японский::N5::Кандзи", 30)]
        elif root_did == 20:
            return [("Японский::N5::Кандзи", 30)]
        return []

    def all_names_and_ids(self):
        class Entry:
            def __init__(self, name, id):
                self.name = name
                self.id = id

        return [Entry(d["name"], d["id"]) for d in self.decks.values()]


class FakeNote:
    def __init__(self, nid, guid, mid, fields_dict, tags):
        self.id = nid
        self.guid = guid
        self.mid = mid
        self._fields = fields_dict
        self.tags = tags

    def items(self):
        return list(self._fields.items())

    def values(self):
        return list(self._fields.values())


class FakeCard:
    def __init__(self, cid, nid, did, ord):
        self.id = cid
        self.nid = nid
        self.did = did
        self.ord = ord


class FakeModelManager:
    def __init__(self):
        self.models = {
            100: {
                "id": 100,
                "name": "Основная",
                "type": 0,
                "sortf": 0,
                "flds": [{"name": "Лицо", "ord": 0}, {"name": "Значение", "ord": 1}],
                "tmpls": [
                    {"name": "Карточка 1", "ord": 0, "qfmt": "{{Лицо}}", "afmt": "{{Значение}}"}
                ],
                "css": ".card { font-size: 20px; }",
            },
            200: {
                "id": 200,
                "name": "Cloze Model",
                "type": 1,
                "sortf": 0,
                "flds": [{"name": "Text", "ord": 0}, {"name": "Extra", "ord": 1}],
                "tmpls": [
                    {
                        "name": "Cloze 1",
                        "ord": 0,
                        "qfmt": "{{cloze:Text}}",
                        "afmt": "{{cloze:Text}}<br>{{Extra}}",
                    }
                ],
                "css": ".cloze { font-weight: bold; }",
            },
        }

    def get(self, mid):
        return self.models.get(mid)


class FakeMediaManager:
    def dir(self):
        return "/tmp/fake_media"

    def files_in_str(self, mid, string, include_remote=False):
        return []

    def extract_static_media_files(self, mid):
        return []


class FakeCollection:
    def __init__(self):
        self.decks = FakeDeckManager()
        self.models = FakeModelManager()
        self.media = FakeMediaManager()
        self.notes = {
            1001: FakeNote(
                1001,
                "guid-1",
                100,
                {"Лицо": "<b>猫</b>", "Значение": "Кот [sound:cat.mp3]"},
                ["n5"],
            ),
            1002: FakeNote(
                1002,
                "guid-2",
                200,
                {"Text": "{{c1::犬}} (собака)", "Extra": "Животные"},
                ["cloze"],
            ),
        }
        self.cards = {
            2001: FakeCard(2001, 1001, 20, 0),
            2002: FakeCard(2002, 1002, 30, 0),
        }

    def find_cards(self, query):
        if 'deck:"Японский"' in query:
            return [2001, 2002]
        return [2001, 2002]

    def get_card(self, cid):
        return self.cards[cid]

    def get_note(self, nid):
        return self.notes[nid]


class TestCollector(unittest.TestCase):
    def test_get_deck_and_child_ids_regression(self):
        """Regression test для контракта col.decks.children() -> list[(name, DeckId)].

        Тест гарантирует, что возвращаются ID колод, а не их имена.
        """
        col = FakeCollection()
        ids = get_deck_and_child_ids(col, 10)
        # Должен вернуть [10, 20, 30], а не ['Японский', 'Японский::N5', ...]
        self.assertEqual(ids, [10, 20, 30])
        for did in ids:
            self.assertIsInstance(did, int, f"ID колоды должен быть int, а получен {type(did)}")

    def test_collect_export_data(self):
        col = FakeCollection()
        data = collect_export_data(
            col=col,
            root_deck_id=10,
            include_media=False,
            dest_dir="/tmp/export",
        )

        self.assertEqual(data["destination_dir"], "/tmp/export")
        self.assertEqual(data["include_media"], False)
        self.assertEqual(data["root_deck_ids"], ["10"])

        # Проверка структуры decks: root + child + grandchild
        self.assertEqual(len(data["decks"]), 3)
        deck_n5 = next(d for d in data["decks"] if d["id"] == "20")
        self.assertEqual(deck_n5["parent_id"], "10")
        deck_kanji = next(d for d in data["decks"] if d["id"] == "30")
        self.assertEqual(deck_kanji["parent_id"], "20")

        # Проверка notes: именованные поля, сохранение HTML, cloze
        self.assertEqual(len(data["notes"]), 2)
        note_basic = next(n for n in data["notes"] if n["guid"] == "guid-1")
        self.assertEqual(note_basic["note_type"], "Основная")
        self.assertEqual(note_basic["fields"]["Лицо"], "<b>猫</b>")
        self.assertEqual(note_basic["tags"], ["n5"])

        # Проверка note_types: семантика standard и cloze
        self.assertEqual(len(data["note_types"]), 2)
        nt_cloze = next(nt for nt in data["note_types"] if nt["id"] == "200")
        self.assertEqual(nt_cloze["kind"], "cloze")
        nt_std = next(nt for nt in data["note_types"] if nt["id"] == "100")
        self.assertEqual(nt_std["kind"], "standard")

    def test_collect_all_decks_root_identification(self):
        col = FakeCollection()
        data = collect_export_data(
            col=col,
            root_deck_id=None,
            include_media=False,
            dest_dir="/tmp/export",
        )
        # При экспорте всей коллекции root_deck_ids должен содержать только корни
        # (Default и Японский)
        self.assertEqual(sorted(data["root_deck_ids"]), ["1", "10"])
        self.assertEqual(len(data["decks"]), 4)


if __name__ == "__main__":
    unittest.main()
