"""Тесты сбора данных коллекции для экспорта."""

import unittest

from crowdanki_v2.collector import collect_export_data, get_deck_and_child_ids


class FakeDeckManager:
    def __init__(self):
        self.decks = {
            1: {"id": 1, "name": "Default", "desc": ""},
            10: {"id": 10, "name": "Японский", "desc": "Корень"},
            20: {"id": 20, "name": "Японский::N5", "desc": "Подколода"},
        }

    def get(self, did):
        return self.decks.get(did)

    def by_name(self, name):
        for d in self.decks.values():
            if d["name"] == name:
                return d
        return None

    def children(self, root_did):
        if root_did == 10:
            return [(20, "Японский::N5")]
        return []

    def all_names_and_ids(self):
        return []


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
                "flds": [{"name": "Лицо", "ord": 0}, {"name": "Значение", "ord": 1}],
                "tmpls": [
                    {"name": "Карточка 1", "ord": 0, "qfmt": "{{Лицо}}", "afmt": "{{Значение}}"}
                ],
                "css": ".card { font-size: 20px; }",
            }
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
            )
        }
        self.cards = {2001: FakeCard(2001, 1001, 20, 0)}

    def find_cards(self, query):
        if "deck:Японский::N5" in query:
            return [2001]
        return []

    def get_card(self, cid):
        return self.cards[cid]

    def get_note(self, nid):
        return self.notes[nid]


class TestCollector(unittest.TestCase):
    def test_get_deck_and_child_ids(self):
        col = FakeCollection()
        ids = get_deck_and_child_ids(col, 10)
        self.assertEqual(ids, [10, 20])

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

        # Проверка структуры decks
        self.assertEqual(len(data["decks"]), 2)
        deck_n5 = next(d for d in data["decks"] if d["id"] == "20")
        self.assertEqual(deck_n5["parent_id"], "10")

        # Проверка notes: именованные поля и сохранение HTML
        self.assertEqual(len(data["notes"]), 1)
        note = data["notes"][0]
        self.assertEqual(note["guid"], "guid-1")
        self.assertEqual(note["note_type"], "Основная")
        self.assertEqual(note["fields"]["Лицо"], "<b>猫</b>")
        self.assertEqual(note["tags"], ["n5"])

        # Проверка cards: note_guid и deck_id
        self.assertEqual(len(data["cards"]), 1)
        card = data["cards"][0]
        self.assertEqual(card["note_guid"], "guid-1")
        self.assertEqual(card["deck_id"], "20")


if __name__ == "__main__":
    unittest.main()
