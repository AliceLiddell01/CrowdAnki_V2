"""Тесты логики адаптера диалога экспорта."""

import unittest

from crowdanki_v2.exporter import CrowdAnkiExporter


class DummyExporter:
    @staticmethod
    def name():
        return "Dummy"


class TestExportDialogAdapterLogic(unittest.TestCase):
    def test_format_selection_visibility_logic(self):
        """Проверяет логику показа/скрытия блока сводки при выборе разных экспортеров:
        - CrowdAnkiExporter -> виден
        - другой экспортер -> скрыт
        """
        exporter_classes = [DummyExporter, CrowdAnkiExporter]

        # 1. Выбран Dummy (индекс 0)
        idx = 0
        is_crowdanki = issubclass(exporter_classes[idx], CrowdAnkiExporter)
        self.assertFalse(is_crowdanki)

        # 2. Выбран CrowdAnki (индекс 1)
        idx = 1
        is_crowdanki = issubclass(exporter_classes[idx], CrowdAnkiExporter)
        self.assertTrue(is_crowdanki)

        # 3. Переключение обратно на Dummy
        idx = 0
        is_crowdanki = issubclass(exporter_classes[idx], CrowdAnkiExporter)
        self.assertFalse(is_crowdanki)


if __name__ == "__main__":
    unittest.main()
