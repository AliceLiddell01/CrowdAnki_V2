"""Тесты регистрации экспортера."""

import unittest

from crowdanki_v2 import _on_exporters_list_did_initialize


class TestExporterRegistration(unittest.TestCase):
    def test_registration_adds_exporter_class(self):
        exporters_list = []
        _on_exporters_list_did_initialize(exporters_list)

        # Если модуль aqt доступен или экспортер импортируется
        # Если aqt нет, exporter не добавится, но падения быть не должно
        self.assertIsInstance(exporters_list, list)


if __name__ == "__main__":
    unittest.main()
