"""Модульные тесты для модуля запуска ядра core_runner."""

from __future__ import annotations

import subprocess
import unittest

from crowdanki_v2.core_runner import (
    CoreExecutionError,
    format_process_failure_message,
    get_subprocess_popen_kwargs,
    validate_import_plan_result,
)


class TestCoreRunner(unittest.TestCase):
    def test_subprocess_popen_kwargs_windows(self):
        """Проверяет наличие creationflags CREATE_NO_WINDOW на Windows."""
        kwargs = get_subprocess_popen_kwargs(system_name="Windows")
        self.assertIn("creationflags", kwargs)
        expected_flag = getattr(subprocess, "CREATE_NO_WINDOW", 0x08000000)
        self.assertEqual(kwargs["creationflags"], expected_flag)

    def test_subprocess_popen_kwargs_unix(self):
        """Проверяет отсутствие Windows-специфичных флагов на Linux и macOS."""
        kwargs_linux = get_subprocess_popen_kwargs(system_name="Linux")
        self.assertEqual(kwargs_linux, {})

        kwargs_darwin = get_subprocess_popen_kwargs(system_name="Darwin")
        self.assertEqual(kwargs_darwin, {})

    def test_format_process_failure_message_windows_interruption(self):
        """Проверяет понятную диагностику для кода прерывания 3221225786 (0xC000013A)."""
        msg = format_process_failure_message(3221225786, "")
        self.assertIn("0xC000013A", msg)
        self.assertIn("был прерван пользователем или закрыт системой", msg)

        # Знаковый int32 эквивалент
        msg_signed = format_process_failure_message(-1073741510, "")
        self.assertIn("0xC000013A", msg_signed)

    def test_format_process_failure_message_signals(self):
        """Проверяет диагностику сигналов завершения."""
        msg_sigint = format_process_failure_message(-2, "")
        self.assertIn("был прерван пользователем или закрыт системой", msg_sigint)

        msg_sigterm = format_process_failure_message(-15, "")
        self.assertIn("принудительно остановлен", msg_sigterm)

    def test_format_process_failure_message_stderr(self):
        """Проверяет вывод бизнес-ошибки при наличии stderr."""
        msg = format_process_failure_message(3, "каталог не пуст")
        self.assertEqual(msg, "Ошибка при выполнении экспорта: каталог не пуст")

    def test_format_process_failure_message_fallback(self):
        """Проверяет fallback-диагностику без stderr."""
        msg = format_process_failure_message(1, "")
        self.assertEqual(msg, "Go-ядро непредвиденно завершилось (код возврата: 1)")

    def test_validate_import_plan_result_success(self):
        """Проверяет успешную валидацию корректного ответа ImportPlan."""
        data = {
            "can_apply": True,
            "root_decks": ["Deck1"],
            "summary": {
                "total_decks": 1,
                "total_notes": 1,
                "total_cards": 1,
                "total_note_types": 1,
                "created_decks": 0,
                "deleted_decks": 0,
                "created_notes": 0,
                "updated_notes": 1,
                "deleted_notes": 0,
                "created_cards": 0,
                "moved_cards": 0,
                "deleted_cards": 0,
                "created_note_types": 0,
                "updated_note_types": 0,
                "added_media": 0,
                "same_media": 0,
                "conflict_media": 0,
                "missing_media": 0,
                "total_conflicts": 0,
            },
            "conflicts": [],
            "warnings": ["Предупреждение"],
            "deck_ops": [],
            "note_type_ops": [],
            "note_ops": [],
            "card_ops": [],
            "media_ops": [],
        }
        res = validate_import_plan_result(data)
        self.assertTrue(res["can_apply"])
        self.assertEqual(res["warnings"], ["Предупреждение"])

    def test_validate_import_plan_result_normalizes_null_and_missing_lists(self):
        """Проверяет замену null и отсутствующих полей-списков на пустые списки."""
        data = {
            "can_apply": False,
            "root_decks": None,
            "summary": {
                "total_decks": 0,
                "total_notes": 0,
                "total_cards": 0,
                "total_note_types": 0,
                "created_decks": 0,
                "deleted_decks": 0,
                "created_notes": 0,
                "updated_notes": 0,
                "deleted_notes": 0,
                "created_cards": 0,
                "moved_cards": 0,
                "deleted_cards": 0,
                "created_note_types": 0,
                "updated_note_types": 0,
                "added_media": 0,
                "same_media": 0,
                "conflict_media": 0,
                "missing_media": 0,
                "total_conflicts": 0,
            },
            "conflicts": None,
            "warnings": None,
            # deck_ops, note_type_ops, note_ops, card_ops, media_ops отсутствуют
        }
        res = validate_import_plan_result(data)
        self.assertFalse(res["can_apply"])
        self.assertEqual(res["root_decks"], [])
        self.assertEqual(res["conflicts"], [])
        self.assertEqual(res["warnings"], [])
        self.assertEqual(res["deck_ops"], [])
        self.assertEqual(res["note_type_ops"], [])
        self.assertEqual(res["note_ops"], [])
        self.assertEqual(res["card_ops"], [])
        self.assertEqual(res["media_ops"], [])

    def test_validate_import_plan_result_rejects_invalid_type(self):
        """Проверяет выброс CoreExecutionError при невалидных типах полей."""
        with self.assertRaises(CoreExecutionError) as ctx:
            validate_import_plan_result("not a dict")
        self.assertIn("не является объектом", str(ctx.exception))

        with self.assertRaises(CoreExecutionError) as ctx2:
            validate_import_plan_result(
                {
                    "can_apply": True,
                    "summary": {
                        "total_decks": 0,
                        "total_notes": 0,
                        "total_cards": 0,
                        "total_note_types": 0,
                        "created_decks": 0,
                        "deleted_decks": 0,
                        "created_notes": 0,
                        "updated_notes": 0,
                        "deleted_notes": 0,
                        "created_cards": 0,
                        "moved_cards": 0,
                        "deleted_cards": 0,
                        "created_note_types": 0,
                        "updated_note_types": 0,
                        "added_media": 0,
                        "same_media": 0,
                        "conflict_media": 0,
                        "missing_media": 0,
                        "total_conflicts": 0,
                    },
                    "warnings": "not a list",
                }
            )
        self.assertIn("не является списком", str(ctx2.exception))


if __name__ == "__main__":
    unittest.main()
