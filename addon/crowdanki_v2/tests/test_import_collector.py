"""Тесты сбора снимка целевой коллекции для импорта."""

import unittest

from crowdanki_v2.import_collector import collect_dest_snapshot, is_deck_name_in_scope


class FakeDeckEntry:
    def __init__(self, id_val: int, name: str):
        self.id = id_val
        self.name = name


class FakeDeckManager:
    def __init__(self):
        self.decks = {
            1: {"id": 1, "name": "Default", "desc": ""},
            10: {"id": 10, "name": "文法", "desc": "Корень"},
            20: {"id": 20, "name": "文法::N2", "desc": "Подколода N2"},
            99: {"id": 99, "name": "Другое", "desc": "Вне scope"},
        }

    def get(self, did):
        return self.decks.get(did)

    def all_names_and_ids(self):
        return [FakeDeckEntry(d["id"], d["name"]) for d in self.decks.values()]


class FakeCard:
    def __init__(self, cid: int, nid: int, did: int, ord_num: int):
        self.id = cid
        self.nid = nid
        self.did = did
        self.ord = ord_num


class FakeNote:
    def __init__(self, nid: int, guid: str, mid: int, fields: dict, tags: list):
        self.id = nid
        self.guid = guid
        self.mid = mid
        self._fields = fields
        self.tags = tags

    def items(self):
        return list(self._fields.items())


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

    def get(self, mid):
        return self.models.get(mid)


class FakeCollection:
    def __init__(self):
        self.decks = FakeDeckManager()
        self.models = FakeModelManager()
        self.notes = {
            1001: FakeNote(
                1001, "guid-scoped", 100, {"Front": "Слово 1", "Back": "Перевод 1"}, ["tag1"]
            ),
            1002: FakeNote(
                1002, "guid-multi-deck", 100, {"Front": "Слово 2", "Back": "Перевод 2"}, ["tag2"]
            ),
        }
        self.cards = {
            2001: FakeCard(2001, 1001, 20, 0),  # в 文法::N2 (в scope)
            2002: FakeCard(2002, 1002, 20, 0),  # в 文法::N2 (в scope)
            2003: FakeCard(2003, 1002, 99, 1),  # в Другое (вне scope!)
        }

    def find_cards(self, query: str):
        if "deck:文法" in query:
            return [2001, 2002]
        return list(self.cards.keys())

    def get_card(self, cid: int):
        return self.cards[cid]

    def get_note(self, nid: int):
        return self.notes[nid]

    def card_ids_of_note(self, nid: int):
        return [cid for cid, c in self.cards.items() if c.nid == nid]


class TestImportCollector(unittest.TestCase):
    def test_is_deck_name_in_scope(self):
        roots = ["文法"]
        self.assertTrue(is_deck_name_in_scope("文法", roots))
        self.assertTrue(is_deck_name_in_scope("文法::N2", roots))
        self.assertTrue(is_deck_name_in_scope("文法::N2::Урок 1", roots))
        self.assertFalse(is_deck_name_in_scope("Другое", roots))
        self.assertFalse(is_deck_name_in_scope("文法_другая", roots))

    def test_collect_dest_snapshot_out_of_scope_marking(self):
        col = FakeCollection()
        snap = collect_dest_snapshot(col, ["文法"])

        # Проверяем колоды
        self.assertEqual(len(snap["decks"]), 4)

        # Проверяем заметки
        note_guids = [n["guid"] for n in snap["notes"]]
        self.assertIn("guid-scoped", note_guids)
        self.assertIn("guid-multi-deck", note_guids)

        # Проверяем карточки
        cards_by_id = {c["id"]: c for c in snap["cards"]}
        self.assertEqual(len(cards_by_id), 3)

        # 2001 и 2002 в scope
        self.assertTrue(cards_by_id["2001"]["in_scope"])
        self.assertTrue(cards_by_id["2002"]["in_scope"])

        # 2003 вне scope (принадлежит guid-multi-deck, но в колоде Другое)
        self.assertFalse(cards_by_id["2003"]["in_scope"])
        self.assertEqual(cards_by_id["2003"]["deck_name"], "Другое")

        # Проверяем сбор типов заметок
        self.assertEqual(len(snap["note_types"]), 1)
        self.assertEqual(snap["note_types"][0]["name"], "Базовый")


if __name__ == "__main__":
    unittest.main()
