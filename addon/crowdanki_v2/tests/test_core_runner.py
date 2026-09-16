"""Модульные тесты для модуля запуска ядра core_runner."""

from __future__ import annotations

import subprocess
import unittest

from crowdanki_v2.core_runner import (
    format_process_failure_message,
    get_subprocess_popen_kwargs,
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


if __name__ == "__main__":
    unittest.main()
