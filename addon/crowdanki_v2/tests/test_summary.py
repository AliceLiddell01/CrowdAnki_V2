"""Тесты форматирования сообщений и предварительной сводки."""

import unittest

from crowdanki_v2.summary import (
    format_bytes,
    format_export_result_message,
    format_summary_text,
    format_throughput,
)


class TestSummaryFormatting(unittest.TestCase):
    def test_format_bytes(self):
        self.assertEqual(format_bytes(500), "500 Б")
        self.assertEqual(format_bytes(1024), "1,0 КБ")
        self.assertEqual(format_bytes(1024 * 1024 * 5), "5,0 МБ")
        self.assertEqual(format_bytes(1024 * 1024 * 1024 * 2), "2,00 ГБ")

    def test_format_throughput(self):
        # 10 МБ за 1 секунду
        bytes_count = 10 * 1024 * 1024
        self.assertEqual(format_throughput(bytes_count, 1000), "10,0 МБ/с")
        self.assertEqual(format_throughput(0, 1000), "")
        self.assertEqual(format_throughput(bytes_count, 0), "")

    def test_format_summary_text_with_media(self):
        text = format_summary_text(
            decks_count=2,
            notes_count=10,
            cards_count=20,
            note_types_count=1,
            media_count=5,
            media_bytes=1024 * 1024,
            include_media=True,
            missing_media_count=2,
        )
        self.assertIn("Колод:          2", text)
        self.assertIn("Заметок:        10", text)
        self.assertIn("Карточек:       20", text)
        self.assertIn("Типов заметок:  1", text)
        self.assertIn("Медиафайлов:    5", text)
        self.assertIn("Размер медиа:   1,0 МБ", text)
        self.assertIn("Не найдено медиа: 2", text)

    def test_format_summary_text_media_disabled(self):
        text = format_summary_text(
            decks_count=1,
            notes_count=5,
            cards_count=5,
            note_types_count=1,
            media_count=0,
            media_bytes=0,
            include_media=False,
        )
        self.assertIn("Медиафайлы:    не включены", text)
        self.assertNotIn("Размер медиа", text)

    def test_format_export_result_message(self):
        result = {
            "elapsed_ms": 2400,
            "notes": 412,
            "cards": 684,
            "media_files": 97,
            "media_bytes": 18 * 1024 * 1024,
            "media_elapsed_ms": 900,
            "missing_media": 3,
        }
        msg = format_export_result_message(result)
        self.assertIn("Экспорт завершён за 2,4 с:", msg)
        self.assertIn("412 заметок, 684 карточек, 97 медиафайлов (18,0 МБ).", msg)
        self.assertIn("Медиа: 0,9 с, 20,0 МБ/с.", msg)
        self.assertIn("Не найдено медиа: 3.", msg)


if __name__ == "__main__":
    unittest.main()
