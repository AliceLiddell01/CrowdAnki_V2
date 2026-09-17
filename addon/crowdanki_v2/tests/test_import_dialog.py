"""Тесты логики отображения и форматирования диалога импорта."""

import unittest

from crowdanki_v2.import_dialog import format_destructive_warning, format_import_result_message


class TestImportDialogLogic(unittest.TestCase):
    def test_format_import_result_message_with_changes(self):
        res = {
            "elapsed_s": 2.345,
            "created_notes": 5,
            "updated_notes": 12,
            "deleted_notes": 3,
            "moved_cards": 4,
            "created_decks": 1,
            "deleted_decks": 2,
            "added_media": 15,
        }
        msg = format_import_result_message(res)
        self.assertIn("2.3 с", msg)
        self.assertIn("Добавлено заметок: 5", msg)
        self.assertIn("Обновлено заметок: 12", msg)
        self.assertIn("Удалено заметок: 3", msg)
        self.assertIn("Перемещено карточек: 4", msg)
        self.assertIn("Создано колод: 1", msg)
        self.assertIn("Удалено колод: 2", msg)
        self.assertIn("Добавлено медиафайлов: 15", msg)

    def test_format_import_result_message_no_changes(self):
        res = {
            "elapsed_s": 0.5,
            "created_notes": 0,
            "updated_notes": 0,
            "deleted_notes": 0,
            "moved_cards": 0,
            "created_decks": 0,
            "deleted_decks": 0,
            "added_media": 0,
        }
        msg = format_import_result_message(res)
        self.assertIn("Изменений нет", msg)

    def test_format_destructive_warning_present(self):
        summary = {
            "deleted_notes": 7,
            "deleted_cards": 8,
            "deleted_decks": 2,
        }
        warn = format_destructive_warning(summary)
        self.assertIsNotNone(warn)
        self.assertIn("7 заметок", warn)
        self.assertIn("8 карточек", warn)
        self.assertIn("2 колод", warn)

    def test_format_destructive_warning_none(self):
        summary = {
            "deleted_notes": 0,
            "deleted_cards": 0,
            "deleted_decks": 0,
        }
        warn = format_destructive_warning(summary)
        self.assertIsNone(warn)


if __name__ == "__main__":
    unittest.main()
