"""Тесты сбора и разрешения медиафайлов."""

import tempfile
import unittest
from pathlib import Path

from crowdanki_v2.media_resolver import get_media_stats, resolve_deck_media


class FakeMediaManager:
    def __init__(self):
        self.files_in_str_calls = []

    def files_in_str(self, mid: int, string: str, include_remote: bool = False):
        self.files_in_str_calls.append((mid, string))
        found = []
        if "cat.png" in string:
            found.append("cat.png")
        if "cat.mp3" in string:
            found.append("cat.mp3")
        return found

    def extract_static_media_files(self, mid: int):
        if mid == 1:
            return ["_font.woff2"]
        return []


class FakeCollection:
    def __init__(self):
        self.media = FakeMediaManager()


class TestMediaResolver(unittest.TestCase):
    def test_resolve_deck_media_deduplication_and_statics(self):
        col = FakeCollection()
        notes_fields = [
            (1, ["<img src='cat.png'>", "Кот [sound:cat.mp3]"]),
            (1, ["Повтор [sound:cat.mp3]", "Тоже кот <img src='cat.png'>"]),
        ]
        notetype_ids = [1]

        media_files = resolve_deck_media(col, notes_fields, notetype_ids)
        # Ожидается дедупликация cat.png, cat.mp3 плюс статический _font.woff2
        self.assertEqual(media_files, ["_font.woff2", "cat.mp3", "cat.png"])

    def test_get_media_stats(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            f1 = tmp_path / "f1.txt"
            f1.write_bytes(b"12345")  # 5 байт

            existing_cnt, total_bytes, missing_cnt = get_media_stats(
                str(tmp_path), ["f1.txt", "missing.mp3"]
            )
            self.assertEqual(existing_cnt, 1)
            self.assertEqual(total_bytes, 5)
            self.assertEqual(missing_cnt, 1)


if __name__ == "__main__":
    unittest.main()
