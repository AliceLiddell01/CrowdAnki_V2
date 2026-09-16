"""Адаптер совместимости для стандартного окна ExportDialog в Anki.

Позволяет для экспортера CrowdAnki V2:
1. Заменять выбор файла на выбор каталога без привязки к имени колоды.
2. Отображать блок предварительной сводки 'Будет экспортировано' с асинхронным подсчетом.

Не модифицирует поведение диалога для других экспортеров.
"""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

from aqt.operations import QueryOp
from aqt.qt import (
    QFileDialog,
    QGroupBox,
    QLabel,
    QVBoxLayout,
)

from .collector import get_deck_and_child_ids
from .exporter import CrowdAnkiExporter
from .media_resolver import get_media_stats, resolve_deck_media
from .summary import format_summary_text

if TYPE_CHECKING:
    from anki.collection import Collection
    from aqt.import_export.exporting import ExportDialog


class ExportDialogAdapter:
    """Изолированный адаптер расширения возможностей ExportDialog для CrowdAnki V2."""

    def __init__(self, dialog: ExportDialog) -> None:
        self.dialog = dialog
        self.summary_box: QGroupBox | None = None
        self.summary_label: QLabel | None = None
        self._current_request_id: int = 0

        self._install_ui()
        self._hook_dialog_methods()

    def _install_ui(self) -> None:
        """Добавляет блок сводки под чекбоксами диалога экспорта."""
        frm = self.dialog.frm
        # Ищем компоновку с чекбоксами (второй элемент вертикального layout диалога)
        layout = self.dialog.layout()
        if not layout:
            return

        self.summary_box = QGroupBox("Будет экспортировано", self.dialog)
        box_layout = QVBoxLayout()
        self.summary_label = QLabel("Подсчёт…", self.summary_box)
        box_layout.addWidget(self.summary_label)
        self.summary_box.setLayout(box_layout)
        self.summary_box.setVisible(False)

        # Вставляем блок сводки перед вертикальным распорщиком
        layout.insertWidget(2, self.summary_box)

        # Подписываемся на смену колоды и изменение чекбокса медиа
        frm.deck.currentIndexChanged.connect(self._on_options_changed)
        frm.includeMedia.stateChanged.connect(self._on_options_changed)

    def _hook_dialog_methods(self) -> None:
        """Перехватывает методы get_out_path и exporter_changed."""
        orig_exporter_changed = self.dialog.exporter_changed
        orig_get_out_path = self.dialog.get_out_path

        def patched_exporter_changed(idx: int) -> None:
            orig_exporter_changed(idx)
            is_crowdanki = isinstance(self.dialog.exporter, CrowdAnkiExporter)
            if self.summary_box:
                self.summary_box.setVisible(is_crowdanki)
            if is_crowdanki:
                self._update_summary_async()

        def patched_get_out_path() -> str | None:
            if isinstance(self.dialog.exporter, CrowdAnkiExporter):
                # Для CrowdAnki V2 выбираем каталог
                dest_dir = QFileDialog.getExistingDirectory(
                    self.dialog,
                    "Выберите каталог для экспорта CrowdAnki V2",
                    "",
                    QFileDialog.Option.ShowDirsOnly,
                )
                if not dest_dir:
                    return None
                return os.path.normpath(dest_dir)
            return orig_get_out_path()

        self.dialog.exporter_changed = patched_exporter_changed  # type: ignore[assignment]
        self.dialog.get_out_path = patched_get_out_path  # type: ignore[assignment]

        # Если диалог уже инициализирован на CrowdAnki V2
        if isinstance(self.dialog.exporter, CrowdAnkiExporter):
            if self.summary_box:
                self.summary_box.setVisible(True)
            self._update_summary_async()

    def _on_options_changed(self, *_) -> None:
        if isinstance(self.dialog.exporter, CrowdAnkiExporter):
            self._update_summary_async()

    def _update_summary_async(self) -> None:
        """Запускает асинхронный фоновый подсчет сводки для выбранной колоды."""
        if not self.summary_label:
            return

        self._current_request_id += 1
        request_id = self._current_request_id
        self.summary_label.setText("Подсчёт…")

        include_media = self.dialog.frm.includeMedia.isChecked()
        deck_id = self.dialog.current_deck_id()

        def calculate(col: Collection) -> tuple[int, int, int, int, int, int, int]:
            if deck_id is not None:
                deck_ids = get_deck_and_child_ids(col, deck_id)
            else:
                all_deck_entries = col.decks.all_names_and_ids()
                deck_ids = [e.id for e in all_deck_entries]

            decks_count = len(deck_ids)

            card_ids: list[int] = []
            for did in deck_ids:
                d = col.decks.get(did)
                if d:
                    card_ids.extend(col.find_cards(f'"deck:{d["name"]}"'))

            card_ids = sorted(set(card_ids))
            cards_count = len(card_ids)

            seen_nids = set()
            for cid in card_ids:
                c = col.get_card(cid)
                seen_nids.add(c.nid)

            notes_count = len(seen_nids)
            used_mids = set()
            notes_fields = []

            for nid in seen_nids:
                n = col.get_note(nid)
                used_mids.add(n.mid)
                notes_fields.append((n.mid, list(n.values())))

            note_types_count = len(used_mids)

            media_count = 0
            media_bytes = 0
            missing_count = 0

            if include_media:
                media_files = resolve_deck_media(col, notes_fields, list(used_mids))
                media_dir = col.media.dir() if hasattr(col.media, "dir") else ""
                existing_cnt, total_b, miss_cnt = get_media_stats(media_dir, media_files)
                media_count = existing_cnt
                media_bytes = total_b
                missing_count = miss_cnt

            return (
                decks_count,
                notes_count,
                cards_count,
                note_types_count,
                media_count,
                media_bytes,
                missing_count,
            )

        def on_success(counts: tuple[int, int, int, int, int, int, int]) -> None:
            # Предотвращение гонки: обновляем UI только если это самый свежий запрос
            if request_id != self._current_request_id or not self.summary_label:
                return

            text = format_summary_text(
                decks_count=counts[0],
                notes_count=counts[1],
                cards_count=counts[2],
                note_types_count=counts[3],
                media_count=counts[4],
                media_bytes=counts[5],
                include_media=include_media,
                missing_media_count=counts[6],
            )
            self.summary_label.setText(text)

        def on_failure(_: Exception) -> None:
            if request_id == self._current_request_id and self.summary_label:
                self.summary_label.setText("Не удалось рассчитать сводку")

        QueryOp(
            parent=self.dialog,
            op=calculate,
            success=on_success,
        ).failure(on_failure).run_in_background()


def setup_export_dialog_adapter(dialog: ExportDialog) -> None:
    """Инициализирует адаптер для переданного экземпляра ExportDialog."""
    ExportDialogAdapter(dialog)
