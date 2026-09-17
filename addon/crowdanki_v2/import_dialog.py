"""Диалоговое окно импорта CrowdAnki V2 на базе стандартных компонентов Qt."""

from __future__ import annotations

import json
import logging
import os
from typing import TYPE_CHECKING, Any

try:
    from aqt.operations import QueryOp
    from aqt.qt import (
        QAbstractItemView,
        QButtonGroup,
        QCheckBox,
        QComboBox,
        QCursor,
        QDialog,
        QFileDialog,
        QGroupBox,
        QHBoxLayout,
        QHeaderView,
        QLabel,
        QProgressBar,
        QPushButton,
        Qt,
        QTreeWidget,
        QTreeWidgetItem,
        QVBoxLayout,
    )
    from aqt.utils import showWarning, tooltip
except ImportError:
    # Заглушки для среды без Qt/Anki
    QueryOp = None  # type: ignore[assignment]
    QAbstractItemView = None  # type: ignore[assignment]
    QButtonGroup = None  # type: ignore[assignment]
    QCheckBox = None  # type: ignore[assignment]
    QComboBox = None  # type: ignore[assignment]
    QCursor = None  # type: ignore[assignment]
    QDialog = object  # type: ignore[assignment,misc]
    QFileDialog = None  # type: ignore[assignment]
    QGroupBox = None  # type: ignore[assignment]
    QHBoxLayout = None  # type: ignore[assignment]
    QHeaderView = None  # type: ignore[assignment]
    QLabel = None  # type: ignore[assignment]
    QProgressBar = None  # type: ignore[assignment]
    QPushButton = None  # type: ignore[assignment]
    QTreeWidget = None  # type: ignore[assignment]
    QTreeWidgetItem = None  # type: ignore[assignment]
    QVBoxLayout = None  # type: ignore[assignment]
    Qt = None  # type: ignore[assignment]
    showWarning = None  # type: ignore[assignment]
    tooltip = None  # type: ignore[assignment]

from .core_runner import CoreExecutionError, run_core_import_plan
from .import_applier import apply_import_plan
from .import_collector import collect_dest_snapshot
from .summary import format_bytes

if TYPE_CHECKING:
    from aqt.main import AnkiQt

logger = logging.getLogger("crowdanki_v2")


def format_import_result_message(res: dict[str, Any]) -> str:
    """Формирует понятное и лаконичное итоговое сообщение об успешном импорте."""
    lines = [f"Импорт CrowdAnki V2 успешно завершён за {res.get('elapsed_s', 0):.1f} с."]
    details: list[str] = []

    if res.get("created_notes", 0) > 0:
        details.append(f"Добавлено заметок: {res['created_notes']}")
    if res.get("updated_notes", 0) > 0:
        details.append(f"Обновлено заметок: {res['updated_notes']}")
    if res.get("deleted_notes", 0) > 0:
        details.append(f"Удалено заметок: {res['deleted_notes']}")
    if res.get("moved_cards", 0) > 0:
        details.append(f"Перемещено карточек: {res['moved_cards']}")
    if res.get("created_decks", 0) > 0:
        details.append(f"Создано колод: {res['created_decks']}")
    if res.get("deleted_decks", 0) > 0:
        details.append(f"Удалено колод: {res['deleted_decks']}")
    if res.get("added_media", 0) > 0:
        details.append(f"Добавлено медиафайлов: {res['added_media']}")

    if not details:
        return "Изменений нет. Проект уже полностью соответствует коллекции Anki."

    lines.append("")
    lines.extend(details)
    return "\n".join(lines)


def format_destructive_warning(summary: dict[str, Any]) -> str | None:
    """Формирует текст предупреждения о деструктивных изменениях (удалениях)."""
    del_notes = summary.get("deleted_notes", 0)
    del_cards = summary.get("deleted_cards", 0)
    del_decks = summary.get("deleted_decks", 0)

    if del_notes == 0 and del_cards == 0 and del_decks == 0:
        return None

    parts = []
    if del_notes > 0:
        parts.append(f"{del_notes} заметок")
    if del_cards > 0:
        parts.append(f"{del_cards} карточек")
    if del_decks > 0:
        parts.append(f"{del_decks} колод")

    return f"⚠ Внимание: будут удалены {', '.join(parts)} из управляемого дерева колод."


class ImportDialog(QDialog):
    """Полнофункциональное окно импорта и предварительного просмотра плана CrowdAnki V2."""

    def __init__(self, mw: AnkiQt, initial_dir: str | None = None) -> None:
        super().__init__(mw)
        self.mw = mw
        self.source_dir = initial_dir or ""
        self.current_plan: dict[str, Any] | None = None
        self._is_analyzing = False
        self._is_importing = False
        self._filter_category = "all"
        self._analysis_generation = 0

        self.setWindowTitle("Импорт CrowdAnki V2")
        self.setMinimumWidth(780)
        self.setMinimumHeight(600)

        self._setup_ui()

        if self.source_dir:
            self._start_analysis()

    def _setup_ui(self) -> None:
        main_layout = QVBoxLayout(self)
        main_layout.setSpacing(10)
        main_layout.setContentsMargins(15, 15, 15, 15)

        # 1. Верхний блок: Источник
        source_group = QGroupBox("Источник проекта", self)
        source_layout = QVBoxLayout(source_group)
        source_row = QHBoxLayout()

        self.path_label = QLabel("Папка не выбрана", self)
        self.path_label.setTextInteractionFlags(Qt.TextInteractionFlag.TextSelectableByMouse)
        source_row.addWidget(self.path_label, stretch=1)

        self.select_dir_btn = QPushButton("Выбрать другую папку…", self)
        self.select_dir_btn.clicked.connect(self._on_choose_folder)
        source_row.addWidget(self.select_dir_btn)
        source_layout.addLayout(source_row)

        self.meta_label = QLabel("", self)
        self.meta_label.setStyleSheet("color: gray;")
        source_layout.addWidget(self.meta_label)
        main_layout.addWidget(source_group)

        # 2. Сводка проекта
        self.summary_group = QGroupBox("В проекте", self)
        self.summary_layout = QHBoxLayout(self.summary_group)
        self.summary_text_label = QLabel("Ожидание выбора проекта…", self)
        self.summary_layout.addWidget(self.summary_text_label)
        main_layout.addWidget(self.summary_group)

        # 3. Главная область — план изменений (Бейджи)
        self.changes_group = QGroupBox("Будут внесены изменения", self)
        changes_layout = QVBoxLayout(self.changes_group)

        self.badges_layout = QHBoxLayout()
        self.badge_added = QLabel("+ 0 Добавлено", self)
        self.badge_updated = QLabel("~ 0 Обновлено", self)
        self.badge_moved = QLabel("→ 0 Перемещено", self)
        self.badge_deleted = QLabel("− 0 Удалено", self)
        self.badge_conflicts = QLabel("! 0 Конфликтов", self)

        for badge in (
            self.badge_added,
            self.badge_updated,
            self.badge_moved,
            self.badge_deleted,
            self.badge_conflicts,
        ):
            badge.setStyleSheet(
                "padding: 6px 12px; font-weight: bold; "
                "border-radius: 4px; border: 1px solid palette(mid);"
            )
            self.badges_layout.addWidget(badge)

        changes_layout.addLayout(self.badges_layout)

        # Предупреждение о деструктивных изменениях
        self.warning_label = QLabel("", self)
        self.warning_label.setStyleSheet("color: #d9534f; font-weight: bold; padding: 4px;")
        self.warning_label.setVisible(False)
        changes_layout.addWidget(self.warning_label)

        # Предупреждение о блокирующих конфликтах
        self.conflict_label = QLabel("", self)
        self.conflict_label.setStyleSheet(
            "color: #d9534f; font-weight: bold; padding: 4px; "
            "border: 1px solid #d9534f; border-radius: 4px;"
        )
        self.conflict_label.setVisible(False)
        changes_layout.addWidget(self.conflict_label)

        main_layout.addWidget(self.changes_group)

        # 4. Опция импорта медиа и разрешение конфликтов
        options_layout = QHBoxLayout()
        self.media_checkbox = QCheckBox("Импортировать медиафайлы", self)
        self.media_checkbox.setChecked(True)
        self.media_checkbox.toggled.connect(self._on_media_toggled)
        options_layout.addWidget(self.media_checkbox)

        options_layout.addSpacing(15)
        self.media_conflict_label = QLabel("Конфликты медиа:", self)
        options_layout.addWidget(self.media_conflict_label)

        self.media_conflict_combo = QComboBox(self)
        self.media_conflict_combo.addItem("Пропустить конфликтующие (сохранить файлы Anki)", "skip")
        self.media_conflict_combo.addItem("Блокировать импорт при конфликтах", "block")
        self.media_conflict_combo.addItem("Перезаписать файлы в коллекции Anki", "overwrite")
        self.media_conflict_combo.currentIndexChanged.connect(
            self._on_media_conflict_strategy_changed
        )
        options_layout.addWidget(self.media_conflict_combo)

        options_layout.addStretch(1)
        main_layout.addLayout(options_layout)

        # 5. Интерактивная фильтрация и дерево деталей
        filter_layout = QHBoxLayout()
        filter_label = QLabel("Детали:", self)
        filter_layout.addWidget(filter_label)

        self.filter_btn_group = QButtonGroup(self)
        self.filter_btn_group.setExclusive(True)

        self.filters = [
            ("all", "Все"),
            ("notes", "Заметки"),
            ("cards", "Карточки"),
            ("decks", "Колоды"),
            ("notetypes", "Типы заметок"),
            ("media", "Медиа"),
            ("conflicts", "Конфликты"),
        ]

        for cat_id, cat_name in self.filters:
            btn = QPushButton(cat_name, self)
            btn.setCheckable(True)
            if cat_id == "all":
                btn.setChecked(True)
            self.filter_btn_group.addButton(btn)
            btn.clicked.connect(lambda checked=False, cid=cat_id: self._set_filter(cid))
            filter_layout.addWidget(btn)

        filter_layout.addStretch(1)
        main_layout.addLayout(filter_layout)

        # Дерево деталей
        self.tree = QTreeWidget(self)
        self.tree.setHeaderLabels(["Категория", "Сущность / GUID", "Действие", "Подробности"])
        self.tree.setSelectionMode(QAbstractItemView.SelectionMode.ExtendedSelection)
        self.tree.setRootIsDecorated(False)
        self.tree.header().setSectionResizeMode(0, QHeaderView.ResizeMode.ResizeToContents)
        self.tree.header().setSectionResizeMode(1, QHeaderView.ResizeMode.ResizeToContents)
        self.tree.header().setSectionResizeMode(2, QHeaderView.ResizeMode.ResizeToContents)
        self.tree.header().setSectionResizeMode(3, QHeaderView.ResizeMode.Stretch)
        main_layout.addWidget(self.tree, stretch=1)

        # 6. Нижняя панель: статус, прогресс и кнопки
        bottom_layout = QHBoxLayout()
        self.progress_bar = QProgressBar(self)
        self.progress_bar.setVisible(False)
        self.progress_bar.setRange(0, 0)
        self.status_label = QLabel("", self)
        bottom_layout.addWidget(self.status_label)
        bottom_layout.addWidget(self.progress_bar)
        bottom_layout.addStretch(1)

        self.cancel_btn = QPushButton("Отмена", self)
        self.cancel_btn.clicked.connect(self.reject)
        bottom_layout.addWidget(self.cancel_btn)

        self.import_btn = QPushButton("Импортировать", self)
        self.import_btn.setEnabled(False)
        self.import_btn.clicked.connect(self._on_apply_import)
        bottom_layout.addWidget(self.import_btn)

        main_layout.addLayout(bottom_layout)

    def _on_choose_folder(self) -> None:
        """Открывает диалог выбора каталога проекта CrowdAnki V2."""
        dir_path = QFileDialog.getExistingDirectory(
            self,
            "Выберите каталог проекта CrowdAnki V2",
            self.source_dir or os.path.expanduser("~"),
        )
        if dir_path:
            self.source_dir = os.path.normpath(dir_path)
            self._start_analysis()

    def _start_analysis(self) -> None:
        """Запускает фоновый анализ проекта и построение ImportPlan через QueryOp."""
        if not self.source_dir or self._is_analyzing:
            return

        self._is_analyzing = True
        self._analysis_generation += 1
        current_gen = self._analysis_generation

        self.path_label.setText(self.source_dir)
        self.path_label.setToolTip(self.source_dir)
        self.meta_label.setText("Анализ проекта…")
        self.summary_text_label.setText("Чтение и валидация проекта…")
        self.tree.clear()
        self.warning_label.setVisible(False)
        self.conflict_label.setVisible(False)
        self.import_btn.setEnabled(False)
        self.progress_bar.setVisible(True)
        self.status_label.setText("Анализ проекта…")

        include_media = self.media_checkbox.isChecked()
        media_strategy = (
            self.media_conflict_combo.currentData()
            if hasattr(self, "media_conflict_combo") and self.media_conflict_combo
            else "skip"
        )
        source_dir = self.source_dir

        def background_op(col) -> dict[str, Any]:
            # 1. Чтение маркера crowdanki.json для извлечения корневых колод
            marker_file = os.path.join(source_dir, "crowdanki.json")
            if not os.path.exists(marker_file):
                raise CoreExecutionError(f"В каталоге {source_dir} отсутствует crowdanki.json")
            with open(marker_file, encoding="utf-8") as f:
                manifest_data = json.load(f)
            root_decks = manifest_data.get("root_decks", [])

            # Если в root_decks указаны ID вместо имён, сопоставляем их с именами из decks.json
            decks_file = os.path.join(source_dir, "decks.json")
            if os.path.isfile(decks_file):
                try:
                    with open(decks_file, encoding="utf-8") as df:
                        decks_list = json.load(df)
                    deck_id_to_name = {
                        d.get("id"): d.get("name")
                        for d in decks_list
                        if isinstance(d, dict) and d.get("id") and d.get("name")
                    }
                    deck_names = {
                        d.get("name") for d in decks_list if isinstance(d, dict) and d.get("name")
                    }
                    resolved_roots = []
                    for r in root_decks:
                        if r in deck_names:
                            resolved_roots.append(r)
                        elif r in deck_id_to_name:
                            resolved_roots.append(deck_id_to_name[r])
                        else:
                            resolved_roots.append(r)
                    root_decks = resolved_roots
                except Exception:
                    pass

            # 2. Сбор snapshot текущей коллекции Anki
            dest_snap = collect_dest_snapshot(col, root_decks)

            # 3. Подготовка запроса к Go-ядру
            media_dir = col.media.dir() if hasattr(col.media, "dir") else ""
            req_payload = {
                "source_dir": source_dir,
                "media_dir": media_dir,
                "include_media": include_media,
                "media_conflict_strategy": media_strategy,
                "dest_snapshot": dest_snap,
            }

            return run_core_import_plan(req_payload)

        def on_success(plan: dict[str, Any]) -> None:
            if current_gen != self._analysis_generation:
                return  # Результат устарел, так как пользователь успел сменить папку
            self._is_analyzing = False
            self.progress_bar.setVisible(False)
            self.status_label.setText("")
            self.current_plan = plan
            self._render_plan(plan)

        def on_failure(exception: Exception) -> None:
            if current_gen != self._analysis_generation:
                return
            self._is_analyzing = False
            self.progress_bar.setVisible(False)
            self.status_label.setText("")
            logger.error("Ошибка при анализе проекта CrowdAnki V2: %s", exception, exc_info=True)
            self.meta_label.setText("Ошибка при анализе проекта")
            self.summary_text_label.setText(str(exception))
            self.import_btn.setEnabled(False)
            if isinstance(exception, CoreExecutionError):
                showWarning(str(exception), parent=self)

        QueryOp(parent=self, op=background_op, success=on_success).failure(
            on_failure
        ).run_in_background()

    def _render_plan(self, plan: dict[str, Any]) -> None:
        """Отображает данные плана в элементах интерфейса."""
        summary = plan.get("summary", {})
        root_decks = plan.get("root_decks", [])
        root_str = ", ".join(root_decks) if root_decks else "не определена"

        self.meta_label.setText(f"Колода: {root_str} · Формат: CrowdAnki V2 · схема 1")

        # Сводка проекта
        media_bytes = summary.get("total_media_bytes", 0)
        media_size_str = format_bytes(media_bytes) if media_bytes > 0 else ""
        media_str = f"{summary.get('total_media_files', 0)} медиафайлов"
        if media_size_str:
            media_str += f" · {media_size_str}"

        self.summary_text_label.setText(
            f"{summary.get('total_decks', 0)} колод · "
            f"{summary.get('total_notes', 0)} заметок · "
            f"{summary.get('total_cards', 0)} карточек · "
            f"{summary.get('total_note_types', 0)} тип(ов) заметок · "
            f"{media_str}"
        )

        # Бейджи изменений
        created_total = (
            summary.get("created_notes", 0)
            + summary.get("created_cards", 0)
            + summary.get("created_decks", 0)
            + summary.get("created_note_types", 0)
            + (summary.get("added_media", 0) if self.media_checkbox.isChecked() else 0)
        )
        updated_total = summary.get("updated_notes", 0) + summary.get("updated_note_types", 0)
        moved_total = summary.get("moved_cards", 0)
        deleted_total = (
            summary.get("deleted_notes", 0)
            + summary.get("deleted_cards", 0)
            + summary.get("deleted_decks", 0)
        )
        conflicts_count = self._calculate_active_conflicts_count(plan)
        include_media = self.media_checkbox.isChecked()

        self.badge_added.setText(f"+ {created_total} Добавлено")
        self.badge_updated.setText(f"~ {updated_total} Обновлено")
        self.badge_moved.setText(f"→ {moved_total} Перемещено")
        self.badge_deleted.setText(f"− {deleted_total} Удалено")
        self.badge_conflicts.setText(f"! {conflicts_count} Конфликтов")

        # Деструктивные предупреждения
        destr_msg = format_destructive_warning(summary)
        if destr_msg:
            self.warning_label.setText(destr_msg)
            self.warning_label.setVisible(True)
        else:
            self.warning_label.setVisible(False)

        # Конфликты
        if conflicts_count > 0:
            self.conflict_label.setText(
                f"⛔ Обнаружено {conflicts_count} конфликт(ов). "
                "Импорт заблокирован до их устранения."
            )
            self.conflict_label.setVisible(True)
            self.import_btn.setEnabled(False)
        else:
            self.conflict_label.setVisible(False)
            has_changes = (created_total + updated_total + moved_total + deleted_total) > 0 or (
                include_media and summary.get("added_media", 0) > 0
            )
            self.import_btn.setEnabled(has_changes and plan.get("can_apply", False))
            if not has_changes:
                self.status_label.setText("Изменений нет. Проект уже соответствует колоде Anki.")
            elif len(plan.get("warnings", [])) > 0:
                self.status_label.setText(
                    f"Готово к импорту (предупреждений: {len(plan.get('warnings', []))})"
                )

        # Заполнение дерева деталей
        self._populate_tree()

    def _calculate_active_conflicts_count(self, plan: dict[str, Any]) -> int:
        """Подсчитывает количество активных конфликтов с учетом текущего состояния опции медиа."""
        include_media = self.media_checkbox.isChecked()
        count = 0
        for c in plan.get("conflicts", []):
            if c.get("category") == "media" and not include_media:
                continue
            count += 1
        return count

    def _on_media_toggled(self, checked: bool) -> None:
        """Пересчитывает отображение и перезапускает анализ при переключении чекбокса медиа."""
        if hasattr(self, "media_conflict_label") and self.media_conflict_label:
            self.media_conflict_label.setEnabled(checked)
        if hasattr(self, "media_conflict_combo") and self.media_conflict_combo:
            self.media_conflict_combo.setEnabled(checked)
        if self.source_dir and not self._is_analyzing and not self._is_importing:
            self._start_analysis()
        elif self.current_plan:
            self._render_plan(self.current_plan)

    def _on_media_conflict_strategy_changed(self) -> None:
        """Перезапускает анализ при изменении стратегии разрешения конфликтов медиа."""
        if self.source_dir and not self._is_analyzing and not self._is_importing:
            self._start_analysis()

    def _set_filter(self, category_id: str) -> None:
        """Устанавливает категорию фильтрации дерева изменений."""
        self._filter_category = category_id
        self._populate_tree()

    def _populate_tree(self) -> None:
        """Заполняет дерево деталей в соответствии с текущим фильтром."""
        self.tree.clear()
        if not self.current_plan:
            return

        cat = self._filter_category
        include_media = self.media_checkbox.isChecked()

        # 1. Конфликты
        if cat in ("all", "conflicts"):
            for conf in self.current_plan.get("conflicts", []):
                if conf.get("category") == "media" and not include_media:
                    continue
                item = QTreeWidgetItem(
                    [
                        "Конфликт",
                        conf.get("entity", ""),
                        "! Блокирует",
                        conf.get("message", ""),
                    ]
                )
                self.tree.addTopLevelItem(item)

        # 2. Заметки
        if cat in ("all", "notes"):
            for nop in self.current_plan.get("note_ops", []):
                action = nop.get("action")
                if action == "no-op":
                    continue
                action_text = {
                    "create": "+ Создание",
                    "update": "~ Обновление",
                    "delete": "− Удаление",
                }.get(action, action)
                preview = nop.get("first_field", "")
                item = QTreeWidgetItem(
                    [
                        "Заметка",
                        nop.get("guid", ""),
                        action_text,
                        f"{nop.get('note_type_name', '')}: {preview}",
                    ]
                )
                self.tree.addTopLevelItem(item)

        # 3. Карточки
        if cat in ("all", "cards"):
            for cop in self.current_plan.get("card_ops", []):
                action = cop.get("action")
                if action == "no-op":
                    continue
                action_text = {
                    "create": "+ Создание",
                    "move": "→ Перемещение",
                    "delete": "− Удаление",
                }.get(action, action)
                details = f"ord={cop.get('ord', 0)}"
                if action == "move":
                    details += f" ({cop.get('old_deck', '')} → {cop.get('new_deck', '')})"
                elif action == "create":
                    details += f" (колода: {cop.get('new_deck', '')})"
                elif action == "delete":
                    details += f" (колода: {cop.get('old_deck', '')})"

                item = QTreeWidgetItem(
                    [
                        "Карточка",
                        cop.get("note_guid", ""),
                        action_text,
                        details,
                    ]
                )
                self.tree.addTopLevelItem(item)

        # 4. Колоды
        if cat in ("all", "decks"):
            for dop in self.current_plan.get("deck_ops", []):
                action = dop.get("action")
                if action == "no-op":
                    continue
                action_text = {"create": "+ Создание", "delete": "− Удаление"}.get(action, action)
                item = QTreeWidgetItem(
                    [
                        "Колода",
                        dop.get("name", ""),
                        action_text,
                        dop.get("description", ""),
                    ]
                )
                self.tree.addTopLevelItem(item)

        # 5. Типы заметок
        if cat in ("all", "notetypes"):
            for ntop in self.current_plan.get("note_type_ops", []):
                action = ntop.get("action")
                if action == "no-op":
                    continue
                action_text = {
                    "create": "+ Создание",
                    "update": "~ Обновление",
                    "conflict": "! Конфликт",
                }.get(action, action)
                item = QTreeWidgetItem(
                    [
                        "Тип заметок",
                        ntop.get("name", ""),
                        action_text,
                        ntop.get("details", ""),
                    ]
                )
                self.tree.addTopLevelItem(item)

        # 6. Медиа
        if cat in ("all", "media") and include_media:
            for mop in self.current_plan.get("media_ops", []):
                action = mop.get("action")
                action_text = {
                    "add": "+ Добавление",
                    "same": "= Совпадает",
                    "skip": "↷ Пропущен",
                    "overwrite": "⚠ Перезапись",
                    "conflict": "! Конфликт",
                    "missing": "! Отсутствует",
                }.get(action, action)
                details = f"Размер: {format_bytes(mop.get('size', 0))}"
                if action == "skip":
                    details += " (сохранён существующий файл коллекции Anki)"
                elif action == "overwrite":
                    details += " (файл в коллекции будет перезаписан)"
                elif action == "conflict":
                    details += " (хэш отличается от файла в коллекции)"
                item = QTreeWidgetItem(
                    [
                        "Медиа",
                        mop.get("name", ""),
                        action_text,
                        details,
                    ]
                )
                self.tree.addTopLevelItem(item)

    def _on_apply_import(self) -> None:
        """Запускает фоновое применение утвержденного плана импорта."""
        if not self.current_plan or self._is_importing:
            return

        if not self.current_plan.get("can_apply", False):
            showWarning("Импорт заблокирован: устраните конфликты перед применением.", parent=self)
            return

        self._is_importing = True
        self.import_btn.setEnabled(False)
        self.cancel_btn.setEnabled(False)
        self.select_dir_btn.setEnabled(False)
        self.media_checkbox.setEnabled(False)
        self.progress_bar.setVisible(True)
        self.status_label.setText("Импорт CrowdAnki V2 в коллекцию…")

        include_media = self.media_checkbox.isChecked()
        plan = self.current_plan

        def background_apply(col) -> dict[str, Any]:
            return apply_import_plan(
                col=col,
                plan=plan,
                include_media=include_media,
            )

        def on_success(res: dict[str, Any]) -> None:
            self._is_importing = False
            self.progress_bar.setVisible(False)
            msg = format_import_result_message(res)
            logger.info("Импорт успешно завершен: %s", res)
            if hasattr(self.mw, "reset"):
                self.mw.reset()
            tooltip(msg, parent=self.parent())
            self.accept()

        def on_failure(exception: Exception) -> None:
            self._is_importing = False
            self.progress_bar.setVisible(False)
            self.import_btn.setEnabled(True)
            self.cancel_btn.setEnabled(True)
            self.select_dir_btn.setEnabled(True)
            self.media_checkbox.setEnabled(True)
            self.status_label.setText("Ошибка при импорте")
            logger.error("Ошибка применения импорта CrowdAnki V2: %s", exception, exc_info=True)
            showWarning(f"Ошибка при применении импорта: {exception}", parent=self)

        QueryOp(parent=self, op=background_apply, success=on_success).failure(
            on_failure
        ).run_in_background()
