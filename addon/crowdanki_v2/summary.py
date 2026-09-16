"""Форматирование пользовательских сообщений и предварительной сводки."""

from __future__ import annotations


def format_bytes(num_bytes: int) -> str:
    """Форматирует размер в байтах в человекочитаемую строку."""
    if num_bytes < 1024:
        return f"{num_bytes} Б"
    kb = num_bytes / 1024.0
    if kb < 1024:
        return f"{kb:.1f} КБ".replace(".", ",")
    mb = kb / 1024.0
    if mb < 1024:
        return f"{mb:.1f} МБ".replace(".", ",")
    gb = mb / 1024.0
    return f"{gb:.2f} ГБ".replace(".", ",")


def format_throughput(bytes_count: int, elapsed_ms: int) -> str:
    """Форматирует скорость передачи данных в МБ/с."""
    if elapsed_ms <= 0 or bytes_count <= 0:
        return ""
    seconds = elapsed_ms / 1000.0
    mb = bytes_count / (1024.0 * 1024.0)
    speed = mb / seconds
    return f"{speed:.1f} МБ/с".replace(".", ",")


def format_summary_text(
    decks_count: int,
    notes_count: int,
    cards_count: int,
    note_types_count: int,
    media_count: int,
    media_bytes: int,
    include_media: bool,
    missing_media_count: int = 0,
) -> str:
    """Формирует текст блока 'Будет экспортировано' для диалога экспорта."""
    lines = [
        f"Колод:          {decks_count:,}".replace(",", " "),
        f"Заметок:        {notes_count:,}".replace(",", " "),
        f"Карточек:       {cards_count:,}".replace(",", " "),
        f"Типов заметок:  {note_types_count:,}".replace(",", " "),
    ]

    if include_media:
        lines.append(f"Медиафайлов:    {media_count:,}".replace(",", " "))
        lines.append(f"Размер медиа:   {format_bytes(media_bytes)}")
        if missing_media_count > 0:
            lines.append(f"Не найдено медиа: {missing_media_count}")
    else:
        lines.append("Медиафайлы:    не включены")

    return "\n".join(lines)


def format_export_result_message(result: dict) -> str:
    """Формирует итоговое пользовательское сообщение об окончании экспорта."""
    elapsed_ms = result.get("elapsed_ms", 0)
    elapsed_sec = max(0.1, elapsed_ms / 1000.0)
    sec_str = f"{elapsed_sec:.1f}".replace(".", ",")

    notes = result.get("notes", 0)
    cards = result.get("cards", 0)
    media_files = result.get("media_files", 0)
    media_bytes = result.get("media_bytes", 0)
    media_elapsed_ms = result.get("media_elapsed_ms", 0)
    missing_media = result.get("missing_media", 0)

    msg = f"Экспорт завершён за {sec_str} с:\n{notes} заметок, {cards} карточек"

    if media_files > 0:
        size_str = format_bytes(media_bytes)
        msg += f", {media_files} медиафайлов ({size_str})."
        if media_elapsed_ms > 0:
            media_sec = f"{media_elapsed_ms / 1000.0:.1f}".replace(".", ",")
            throughput = format_throughput(media_bytes, media_elapsed_ms)
            if throughput:
                msg += f"\nМедиа: {media_sec} с, {throughput}."
    else:
        msg += "."

    if missing_media > 0:
        msg += f"\nНе найдено медиа: {missing_media}."

    return msg
