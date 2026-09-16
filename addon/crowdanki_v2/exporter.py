"""Класс экспортера CrowdAnki V2 для интеграции в штатное окно экспорта Anki."""

from __future__ import annotations

import logging
import time
from typing import TYPE_CHECKING, Any

try:
    from anki.collection import DeckIdLimit, NoteIdsLimit
    from aqt import gui_hooks
    from aqt.errors import show_exception
    from aqt.import_export.exporting import Exporter, ExportOptions
    from aqt.operations import QueryOp
    from aqt.utils import showWarning, tooltip
except ImportError:
    # Заглушки для среды без установленного Anki (например, модульное тестирование)
    DeckIdLimit = None  # type: ignore[assignment,misc]
    NoteIdsLimit = None  # type: ignore[assignment,misc]
    gui_hooks = None  # type: ignore[assignment]
    show_exception = None  # type: ignore[assignment]
    ExportOptions = None  # type: ignore[assignment]
    showWarning = None  # type: ignore[assignment]
    tooltip = None  # type: ignore[assignment]

    class QueryOp:  # type: ignore[no-redef]
        def __init__(self, parent: Any = None, op: Any = None, success: Any = None):
            self.parent = parent
            self.op = op
            self.success = success

        def with_progress(self, label: str | None = None) -> QueryOp:
            return self

        def failure(self, on_failure: Any) -> QueryOp:
            return self

        def run_in_background(self) -> None:
            pass

    class Exporter:  # type: ignore[no-redef]
        extension: str
        show_deck_list = False
        show_include_media = False

        @staticmethod
        def name() -> str:
            return ""

        def export(self, mw: Any, options: Any) -> None:
            pass


from .collector import collect_export_data
from .core_runner import CoreExecutionError, run_core_export
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
        parent = options.parent or mw

        # Проверка ограничений экспорта
        if isinstance(options.limit, NoteIdsLimit):
            showWarning(
                "Экспорт выбранных заметок в CrowdAnki V2 в данный момент не поддерживается. "
                "Пожалуйста, выберите экспорт колоды.",
                parent=parent,
            )
            return

        root_deck_id = None
        if isinstance(options.limit, DeckIdLimit):
            root_deck_id = options.limit.deck_id

        logger.info(
            "Запуск экспорта CrowdAnki V2: root_deck_id=%s, out_path=%s, include_media=%s",
            root_deck_id,
            options.out_path,
            options.include_media,
        )

        overall_start_time = time.monotonic()

        def background_op(col) -> dict:
            payload = collect_export_data(
                col=col,
                root_deck_id=root_deck_id,
                include_media=options.include_media,
                dest_dir=options.out_path,
            )
            res = run_core_export(payload)
            # Переопределяем общее время экспорта полным временем
            # от запуска операции до получения результата
            total_elapsed_ms = int((time.monotonic() - overall_start_time) * 1000)
            res["elapsed_ms"] = max(1, total_elapsed_ms)
            return res

        def on_success(result: dict) -> None:
            gui_hooks.exporter_did_export(options, self)
            msg = format_export_result_message(result)
            tooltip(msg, parent=parent)
            logger.info("Экспорт CrowdAnki V2 успешно завершен: %s", result)

        def on_failure(exception: Exception) -> None:
            logger.error("Ошибка при экспорте CrowdAnki V2: %s", exception, exc_info=True)
            if isinstance(exception, CoreExecutionError):
                showWarning(str(exception), parent=parent)
            else:
                show_exception(parent=parent, exception=exception)

        QueryOp(
            parent=parent,
            op=background_op,
            success=on_success,
        ).with_progress(label="Экспорт CrowdAnki V2…").failure(on_failure).run_in_background()
