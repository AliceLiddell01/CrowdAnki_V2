"""Класс экспортера CrowdAnki V2 для интеграции в штатное окно экспорта Anki."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from anki.collection import DeckIdLimit
from aqt import gui_hooks
from aqt.errors import show_exception
from aqt.import_export.exporting import Exporter, ExportOptions, _export_parent
from aqt.operations import QueryOp
from aqt.utils import tooltip

from .collector import collect_export_data
from .core_runner import run_core_export
from .summary import format_export_result_message

if TYPE_CHECKING:
    import aqt.main

logger = logging.getLogger("crowdanki_v2")


class CrowdAnkiExporter(Exporter):
    """Экспортер коллекции Anki в формат каталога CrowdAnki V2."""

    # Служебное расширение; фактический экспорт выполняется в каталог
    extension = "crowdanki"

    # Отображение стандартных контролов Anki ExportDialog
    show_deck_list = True
    show_include_media = True

    # Скрываем нерелевантные или пока не поддерживаемые опции
    show_include_scheduling = False
    show_include_deck_configs = False
    show_include_tags = False
    show_include_html = False
    show_legacy_support = False
    show_include_deck = False
    show_include_notetype = False
    show_include_guid = False

    @staticmethod
    def name() -> str:
        return "CrowdAnki V2 — каталог"

    def export(self, mw: aqt.main.AnkiQt, options: ExportOptions) -> None:
        """Выполняет экспорт в фоновом потоке через QueryOp с вызовом Go-ядра."""
        options = gui_hooks.exporter_will_export(options, self)
        parent = _export_parent(mw, options)

        # Определение выбранной колоды
        root_deck_id = None
        if isinstance(options.limit, DeckIdLimit):
            root_deck_id = options.limit.deck_id

        logger.info(
            "Запуск экспорта CrowdAnki V2: root_deck_id=%s, out_path=%s, include_media=%s",
            root_deck_id,
            options.out_path,
            options.include_media,
        )

        def background_op(col) -> dict:
            payload = collect_export_data(
                col=col,
                root_deck_id=root_deck_id,
                include_media=options.include_media,
                dest_dir=options.out_path,
            )
            return run_core_export(payload)

        def on_success(result: dict) -> None:
            gui_hooks.exporter_did_export(options, self)
            msg = format_export_result_message(result)
            tooltip(msg, parent=parent)
            logger.info("Экспорт CrowdAnki V2 успешно завершен: %s", result)

        def on_failure(exception: Exception) -> None:
            logger.error("Ошибка при экспорте CrowdAnki V2: %s", exception, exc_info=True)
            show_exception(parent=parent, exception=exception)

        QueryOp(
            parent=parent,
            op=background_op,
            success=on_success,
        ).failure(on_failure).run_in_background()
