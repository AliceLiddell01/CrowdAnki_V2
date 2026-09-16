"""Применение утвержденного ImportPlan через публичный API Anki."""

from __future__ import annotations

import logging
import time
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from anki.collection import Collection

logger = logging.getLogger("crowdanki_v2")


class ImportApplicationError(Exception):
    """Исключение при ошибке применения плана импорта."""


def apply_import_plan(
    col: Collection,
    plan: dict[str, Any],
    include_media: bool,
) -> dict[str, Any]:
    """Применяет утвержденный ImportPlan к коллекции Anki в dependency-safe порядке.

    Порядок применения:
    1. Создание бэкапа перед деструктивными изменениями (если есть удаления).
    2. Создание и обновление типов заметок (col.models).
    3. Создание недостающих колод (col.decks.id(create=True)).
    4. Создание новых заметок и распределение их карточек по колодам (col.add_note, col.set_deck).
    5. Обновление полей и тегов существующих заметок (col.update_note)
       с сохранением scheduling state.
    6. Перемещение карточек между колодами (col.set_deck) с сохранением scheduling state.
    7. Удаление устаревших карточек в управляемом scope (col.remove_cards_and_orphaned_notes).
    8. Удаление безопасных осиротевших заметок (col.remove_notes).
    9. Удаление устаревших колод в scope (col.decks.remove).
    10. Добавление медиафайлов (col.media.add_file) при включенной опции.
    11. Сохранение коллекции (col.save).

    Строго соблюдается инвариант: прогресс обучения существующих карточек
    (интервалы, повторения, due, ease, история) никогда не сбрасывается.
    """
    if not plan.get("can_apply", False):
        raise ImportApplicationError(
            "Невозможно применить план импорта: в плане присутствуют неразрешенные конфликты"
        )

    start_time = time.monotonic()
    summary = plan.get("summary", {})

    # 1. Создание резервной копии при наличии деструктивных операций
    has_deletions = (
        summary.get("deleted_notes", 0) > 0
        or summary.get("deleted_cards", 0) > 0
        or summary.get("deleted_decks", 0) > 0
    )
    if has_deletions and hasattr(col, "create_backup"):
        try:
            backup_folder = getattr(col, "backup_folder", "") or ""
            col.create_backup(backup_folder=backup_folder, force=True, wait_for_completion=True)
            logger.info("Создана резервная копия коллекции Anki перед деструктивным импортом")
        except Exception as err:
            logger.warning("Не удалось создать резервную копию перед импортом: %s", err)

    # 2. Создание и обновление типов заметок
    for nt_op in plan.get("note_type_ops", []):
        action = nt_op.get("action")
        name = nt_op.get("name")
        if action == "create":
            nt = col.models.new(name)
            if nt_op.get("kind") == "cloze":
                nt["type"] = 1
            nt["sortf"] = nt_op.get("sortf", 0)
            nt["css"] = nt_op.get("css", "")

            for fld in nt_op.get("fields", []):
                f = col.models.new_field(fld["name"])
                col.models.add_field(nt, f)

            for tmpl in nt_op.get("templates", []):
                t = col.models.new_template(tmpl["name"])
                t["qfmt"] = tmpl.get("qfmt", "")
                t["afmt"] = tmpl.get("afmt", "")
                if "bqfmt" in tmpl:
                    t["bqfmt"] = tmpl["bqfmt"]
                if "bafmt" in tmpl:
                    t["bafmt"] = tmpl["bafmt"]
                col.models.add_template(nt, t)

            if hasattr(col.models, "add_dict"):
                col.models.add_dict(nt)
            else:
                col.models.save(nt)
            logger.info("Создан новый тип заметки: %s", name)

        elif action == "update":
            nt = col.models.by_name(name)
            if nt:
                nt["css"] = nt_op.get("css", nt.get("css", ""))
                existing_fields = {f["name"] for f in nt.get("flds", [])}
                for fld in nt_op.get("fields", []):
                    if fld["name"] not in existing_fields:
                        f = col.models.new_field(fld["name"])
                        col.models.add_field(nt, f)

                for tmpl in nt_op.get("templates", []):
                    found_tmpl = None
                    for existing_t in nt.get("tmpls", []):
                        if existing_t.get("ord") == tmpl.get("ord") or existing_t.get(
                            "name"
                        ) == tmpl.get("name"):
                            found_tmpl = existing_t
                            break
                    if found_tmpl:
                        found_tmpl["qfmt"] = tmpl.get("qfmt", "")
                        found_tmpl["afmt"] = tmpl.get("afmt", "")
                        if "bqfmt" in tmpl:
                            found_tmpl["bqfmt"] = tmpl["bqfmt"]
                        if "bafmt" in tmpl:
                            found_tmpl["bafmt"] = tmpl["bafmt"]
                    else:
                        t = col.models.new_template(tmpl["name"])
                        t["qfmt"] = tmpl.get("qfmt", "")
                        t["afmt"] = tmpl.get("afmt", "")
                        col.models.add_template(nt, t)

                if hasattr(col.models, "update_dict"):
                    col.models.update_dict(nt)
                else:
                    col.models.save(nt)
                logger.info("Обновлен тип заметки: %s", name)

    # 3. Создание колод
    for d_op in plan.get("deck_ops", []):
        if d_op.get("action") == "create":
            col.decks.id(d_op["name"], create=True)
            logger.info("Создана колода: %s", d_op["name"])

    # 4. Создание новых заметок
    note_guid_to_target_deck: dict[str, str] = {}
    for c_op in plan.get("card_ops", []):
        if c_op.get("action") == "create":
            note_guid_to_target_deck[c_op["note_guid"]] = c_op.get("new_deck", "")

    for n_op in plan.get("note_ops", []):
        if n_op.get("action") == "create":
            model = col.models.by_name(n_op["note_type_name"])
            if not model:
                msg = (
                    f"Тип заметки «{n_op['note_type_name']}» не найден при создании "
                    f"заметки {n_op['guid']}"
                )
                raise ImportApplicationError(msg)
            note = col.new_note(model)
            note.guid = n_op["guid"]
            for f_name, f_val in n_op["fields"].items():
                if f_name in note:
                    note[f_name] = f_val
            note.tags = list(n_op.get("tags", []))

            target_deck_name = note_guid_to_target_deck.get(n_op["guid"], "")
            target_deck_id = col.decks.id(target_deck_name, create=True) if target_deck_name else 1
            col.add_note(note, target_deck_id)

    # 5. Обновление существующих заметок (сохраняя прогресс карточек)
    for n_op in plan.get("note_ops", []):
        if n_op.get("action") == "update":
            dest_id = n_op.get("dest_id")
            note_obj = None
            if dest_id:
                try:
                    note_obj = col.get_note(int(dest_id))
                except Exception:
                    note_obj = None
            if not note_obj:
                found_nids = col.find_notes(f"guid:{n_op['guid']}")
                if found_nids:
                    note_obj = col.get_note(found_nids[0])

            if note_obj:
                for f_name, f_val in n_op["fields"].items():
                    if f_name in note_obj:
                        note_obj[f_name] = f_val
                note_obj.tags = list(n_op.get("tags", []))
                col.update_note(note_obj)

    # 6. Перемещение существующих карточек
    for c_op in plan.get("card_ops", []):
        if c_op.get("action") == "move" and c_op.get("dest_id"):
            card_id = int(c_op["dest_id"])
            new_deck_name = c_op["new_deck"]
            new_deck_id = col.decks.id(new_deck_name, create=True)
            col.set_deck([card_id], new_deck_id)

    # 7. Удаление устаревших карточек в управляемом scope
    cards_to_delete = [
        int(c_op["dest_id"])
        for c_op in plan.get("card_ops", [])
        if c_op.get("action") == "delete" and c_op.get("dest_id")
    ]
    if cards_to_delete:
        col.remove_cards_and_orphaned_notes(cards_to_delete)
        logger.info("Удалено устаревших карточек в scope: %d", len(cards_to_delete))

    # 8. Удаление безопасных осиротевших заметок
    notes_to_delete = [
        int(n_op["dest_id"])
        for n_op in plan.get("note_ops", [])
        if n_op.get("action") == "delete" and n_op.get("dest_id")
    ]
    if notes_to_delete:
        existing_nids = []
        for nid in notes_to_delete:
            try:
                if col.get_note(nid):
                    existing_nids.append(nid)
            except Exception:
                pass
        if existing_nids:
            col.remove_notes(existing_nids)
            logger.info("Удалено осиротевших заметок: %d", len(existing_nids))

    # 9. Удаление устаревших колод в scope (дочерние сначала)
    decks_to_delete = [d_op for d_op in plan.get("deck_ops", []) if d_op.get("action") == "delete"]
    decks_to_delete.sort(key=lambda d: len(d["name"]), reverse=True)
    deck_ids_to_remove = []
    for d_op in decks_to_delete:
        if d_op.get("deck_id"):
            deck_ids_to_remove.append(int(d_op["deck_id"]))
    if deck_ids_to_remove:
        col.decks.remove(deck_ids_to_remove)
        logger.info("Удалено устаревших колод в scope: %d", len(deck_ids_to_remove))

    # 10. Добавление медиафайлов
    added_media_count = 0
    if include_media and hasattr(col, "media"):
        for m_op in plan.get("media_ops", []):
            if m_op.get("action") == "add" and m_op.get("source_path"):
                try:
                    col.media.add_file(m_op["source_path"])
                    added_media_count += 1
                except Exception as err:
                    logger.warning("Не удалось добавить медиафайл %s: %s", m_op.get("name"), err)

    if hasattr(col, "save"):
        col.save()

    elapsed_s = time.monotonic() - start_time
    return {
        "elapsed_s": elapsed_s,
        "created_decks": summary.get("created_decks", 0),
        "deleted_decks": summary.get("deleted_decks", 0),
        "created_notes": summary.get("created_notes", 0),
        "updated_notes": summary.get("updated_notes", 0),
        "deleted_notes": summary.get("deleted_notes", 0),
        "created_cards": summary.get("created_cards", 0),
        "moved_cards": summary.get("moved_cards", 0),
        "deleted_cards": summary.get("deleted_cards", 0),
        "created_note_types": summary.get("created_note_types", 0),
        "updated_note_types": summary.get("updated_note_types", 0),
        "added_media": added_media_count,
    }
