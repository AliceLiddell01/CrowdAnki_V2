"""Служба определения медиафайлов коллекции через публичный API Anki."""

from __future__ import annotations

import logging
import os
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from anki.collection import Collection

logger = logging.getLogger("crowdanki_v2")


def resolve_deck_media(
    col: Collection,
    notes_fields: list[tuple[int, list[str]]],
    notetype_ids: list[int],
) -> list[str]:
    """Находит все уникальные локальные медиафайлы, реально используемые заметками и шаблонами.

    notes_fields: список кортежей (notetype_id, [значения полей заметки])
    notetype_ids: список уникальных ID задействованных типов заметок
    """
    media_set: set[str] = set()

    # 1. Поиск медиафайлов в полях заметок с помощью col.media.files_in_str
    for mid, fields in notes_fields:
        for field_text in fields:
            if not field_text:
                continue
            # Проверяем наличие маркеров медиа перед вызовом парсера для оптимизации
            if "[" in field_text or "<" in field_text:
                try:
                    found = col.media.files_in_str(mid, field_text, include_remote=False)
                    for fn in found:
                        if fn:
                            media_set.add(fn)
                except Exception as err:
                    logger.error(
                        "Ошибка извлечения медиафайлов из поля заметки (mid=%s): %s",
                        mid,
                        err,
                        exc_info=True,
                    )
                    raise RuntimeError(
                        f"Не удалось извлечь медиафайлы из содержимого заметки: {err}"
                    ) from err

    # 2. Поиск статических медиаресурсов типов заметок (шрифты, логотипы шаблонов)
    for mid in notetype_ids:
        if hasattr(col.media, "extract_static_media_files"):
            try:
                static_files = col.media.extract_static_media_files(mid)
                for fn in static_files:
                    if fn:
                        media_set.add(fn)
            except Exception as err:
                logger.error(
                    "Ошибка извлечения статических медиафайлов для типа заметки (mid=%s): %s",
                    mid,
                    err,
                    exc_info=True,
                )
                raise RuntimeError(
                    f"Не удалось извлечь статические медиафайлы типа заметки {mid}: {err}"
                ) from err

    return sorted(media_set)


def get_media_stats(media_dir: str, filenames: list[str]) -> tuple[int, int, int]:
    """Быстро вычисляет статистику медиафайлов.

    Возвращает (число существующих, суммарный размер в байтах, число отсутствующих).
    """
    existing_count = 0
    total_bytes = 0
    missing_count = 0

    if not media_dir or not os.path.exists(media_dir):
        return 0, 0, len(filenames)

    for fn in filenames:
        path = os.path.join(media_dir, fn)
        try:
            st = os.stat(path)
            existing_count += 1
            total_bytes += st.st_size
        except OSError:
            missing_count += 1

    return existing_count, total_bytes, missing_count
