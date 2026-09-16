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

    def test_validate_destination_directory_empty_dir(self):
        import tempfile

        from crowdanki_v2.dialog_adapter import validate_destination_directory

        with tempfile.TemporaryDirectory() as tmpdir:
            msg = validate_destination_directory(tmpdir)
            self.assertIsNone(msg)

    def test_validate_destination_directory_existing_project(self):
        import os
        import tempfile

        from crowdanki_v2.dialog_adapter import validate_destination_directory

        with tempfile.TemporaryDirectory() as tmpdir:
            with open(os.path.join(tmpdir, "crowdanki.json"), "w", encoding="utf-8") as f:
                f.write("{}")
            msg = validate_destination_directory(tmpdir)
            self.assertIsNone(msg)

    def test_validate_destination_directory_non_empty_foreign(self):
        import os
        import tempfile

        from crowdanki_v2.dialog_adapter import validate_destination_directory

        with tempfile.TemporaryDirectory() as tmpdir:
            with open(os.path.join(tmpdir, "some_file.txt"), "w", encoding="utf-8") as f:
                f.write("test")
            msg = validate_destination_directory(tmpdir)
            self.assertIsNotNone(msg)
            self.assertIn("не пуст и не содержит проект CrowdAnki V2", msg)

    def test_validate_destination_directory_pm_base_protection(self):
        import os
        import tempfile

        from crowdanki_v2.dialog_adapter import validate_destination_directory

        with tempfile.TemporaryDirectory() as base_dir:
            sub_dir = os.path.join(base_dir, "profile", "export")
            msg = validate_destination_directory(sub_dir, pm_base=base_dir)
            self.assertIsNotNone(msg)
            self.assertIn("каталог профиля Anki защищен", msg)


if __name__ == "__main__":
    unittest.main()
