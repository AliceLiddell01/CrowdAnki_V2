"""Модуль Python-адаптера CrowdAnki V2.

Служит тонким интеграционным слоем между интерфейсом и API Anki
и нативным ядром CrowdAnki V2 на языке Go.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

__version__ = "0.1.0"

if TYPE_CHECKING:
    from aqt.import_export.exporting import Exporter


def _on_exporters_list_did_initialize(exporters: list[type[Exporter]]) -> None:
    """Регистрирует CrowdAnkiExporter в списке экспортеров Anki."""
    try:
        from .exporter import CrowdAnkiExporter

        if CrowdAnkiExporter not in exporters:
            exporters.append(CrowdAnkiExporter)
    except Exception:
        pass


def _init_addon() -> None:
    """Инициализация хуков аддона в среде Anki."""
    try:
        from aqt import gui_hooks
        from aqt.import_export.exporting import ExportDialog

        from .dialog_adapter import setup_export_dialog_adapter

        # 1. Регистрация формата в списке экспортеров
        gui_hooks.exporters_list_did_initialize.append(_on_exporters_list_did_initialize)

        # 2. Подключение изолированного адаптера диалога экспорта
        orig_dialog_init = ExportDialog.__init__

        def hooked_dialog_init(self, *args, **kwargs):
            orig_dialog_init(self, *args, **kwargs)
            try:
                setup_export_dialog_adapter(self)
            except Exception:
                pass

        ExportDialog.__init__ = hooked_dialog_init
    except ImportError:
        # Модули aqt недоступны (например, в изолированной тестовой среде без Anki)
        pass


_init_addon()
