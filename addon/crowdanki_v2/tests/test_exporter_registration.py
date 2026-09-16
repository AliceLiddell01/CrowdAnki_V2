"""Тесты регистрации экспортера."""

import unittest

from crowdanki_v2 import _on_exporters_list_did_initialize


class TestExporterRegistration(unittest.TestCase):
    def test_registration_behavior(self):
        """Проверяет поведенческие инварианты регистрации экспортера:
        1. До вызова exporter отсутствует.
        2. После вызова класс экспортера добавлен ровно 1 раз.
        3. Повторный вызов не дублирует регистрацию.
        """
        exporters_list = []

        # Первый вызов
        _on_exporters_list_did_initialize(exporters_list)
        self.assertEqual(len(exporters_list), 1)
        exporter_cls = exporters_list[0]
        self.assertEqual(exporter_cls.name(), "CrowdAnki V2 — каталог")
        self.assertEqual(exporter_cls.extension, "crowdanki")

        # Повторный вызов не должен дублировать класс
        _on_exporters_list_did_initialize(exporters_list)
        self.assertEqual(len(exporters_list), 1)


if __name__ == "__main__":
    unittest.main()
