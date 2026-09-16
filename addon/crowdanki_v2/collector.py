"""Сбор и нормализация данных коллекции Anki для экспорта в CrowdAnki V2."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from anki.collection import Collection
    from anki.decks import DeckId


def get_deck_and_child_ids(col: Collection, root_deck_id: DeckId) -> list[DeckId]:
    """Возвращает список ID корневой колоды и всех ее дочерних колод любого уровня вложенности.

    В соответствии с публичным контрактом Anki 26.09.2 col.decks.children() возвращает
    список кортежей (name, id), а не (id, name).
    """
    child_names_and_ids = col.decks.children(root_deck_id)
    deck_ids = [root_deck_id]
    for _name, did in child_names_and_ids:
        deck_ids.append(did)
    return deck_ids


def collect_export_data(
    col: Collection,
    root_deck_id: DeckId | None,
    include_media: bool,
    dest_dir: str,
) -> dict[str, Any]:
    """Собирает полное DTO коллекции для передачи в Go-ядро экспорта.

    Использует только публичный API Anki: col.decks, col.find_cards, col.get_note, col.models.
    Прямые запросы к SQLite не выполняются.
    """
    from .media_resolver import resolve_deck_media

    # 1. Определение списка экспортируемых колод
    if root_deck_id is not None:
        target_deck_ids = get_deck_and_child_ids(col, root_deck_id)
        root_deck_ids_str = [str(root_deck_id)]
    else:
        # Экспорт всех колод коллекции: root_deck_ids содержит только настоящие корневые колоды
        all_deck_entries = col.decks.all_names_and_ids()
        target_deck_ids = [entry.id for entry in all_deck_entries]
        root_deck_ids_str = []
        for entry in all_deck_entries:
            if "::" not in entry.name:
                root_deck_ids_str.append(str(entry.id))

    target_deck_ids_set = set(target_deck_ids)

    decks_dto: list[dict[str, Any]] = []
    for did in target_deck_ids:
        deck_dict = col.decks.get(did)
        if not deck_dict:
            continue
        parent_id_str = None
        # Вычисляем родительскую колоду по имени (разделитель ::)
        name = deck_dict["name"]
        parts = name.split("::")
        if len(parts) > 1:
            parent_name = "::".join(parts[:-1])
            parent_deck = col.decks.by_name(parent_name)
            if parent_deck and parent_deck["id"] in target_deck_ids_set:
                parent_id_str = str(parent_deck["id"])

        decks_dto.append(
            {
                "id": str(did),
                "parent_id": parent_id_str,
                "name": name,
                "description": deck_dict.get("desc", ""),
            }
        )

    # 2. Поиск карточек и заметок в выбранных колодах
    card_ids: list[int] = []
    if root_deck_id is not None:
        root_deck_dict = col.decks.get(root_deck_id)
        if root_deck_dict:
            # Запрос 'deck:RootDeck' в Anki автоматически включает все подколоды
            card_ids = col.find_cards(f'"deck:{root_deck_dict["name"]}"')
    else:
        # Для всей коллекции
        card_ids = col.find_cards("")

    card_ids = sorted(set(card_ids))

    cards_dto: list[dict[str, Any]] = []
    seen_note_ids: set[int] = set()

    for cid in card_ids:
        card = col.get_card(cid)
        seen_note_ids.add(card.nid)
        cards_dto.append(
            {
                "note_guid": "",  # Будет заполнено после получения note
                "card_id": str(cid),
                "ord": card.ord,
                "deck_id": str(card.did),
                "_nid": card.nid,
            }
        )

    # 3. Сбор заметок и привязка их GUID к карточкам
    notes_dto: list[dict[str, Any]] = []
    used_model_ids: set[int] = set()
    nid_to_guid: dict[int, str] = {}
    notes_fields_for_media: list[tuple[int, list[str]]] = []

    for nid in sorted(seen_note_ids):
        note = col.get_note(nid)
        guid = note.guid
        nid_to_guid[nid] = guid
        mid = note.mid
        used_model_ids.add(mid)

        model = col.models.get(mid)
        model_name = model["name"] if model else ""

        # Преобразуем поля заметки в именованный словарь {"ИмяПоля": "Значение"}
        # note.items() возвращает пары (field_name, field_value) без изменения HTML
        fields_dict: dict[str, str] = dict(note.items())
        field_values: list[str] = list(note.values())
        notes_fields_for_media.append((mid, field_values))

        notes_dto.append(
            {
                "guid": guid,
                "note_type_id": str(mid),
                "note_type": model_name,
                "fields": fields_dict,
                "tags": sorted(note.tags),
            }
        )

    # Обновляем note_guid в карточках
    for c in cards_dto:
        c["note_guid"] = nid_to_guid.get(c.pop("_nid"), "")

    # 4. Сбор спецификаций задействованных типов заметок с семантикой типа (kind/type)
    note_types_dto: list[dict[str, Any]] = []
    for mid in sorted(used_model_ids):
        model = col.models.get(mid)
        if not model:
            continue

        model_type_code = model.get("type", 0)
        kind = "cloze" if model_type_code == 1 else "standard"
        sortf = model.get("sortf", 0)

        fields_list: list[dict[str, Any]] = []
        for fld in model.get("flds", []):
            fields_list.append(
                {
                    "name": fld["name"],
                    "ord": fld["ord"],
                }
            )

        templates_list: list[dict[str, Any]] = []
        for tmpl in model.get("tmpls", []):
            templates_list.append(
                {
                    "name": tmpl["name"],
                    "ord": tmpl["ord"],
                    "qfmt": tmpl.get("qfmt", ""),
                    "afmt": tmpl.get("afmt", ""),
                    "bqfmt": tmpl.get("bqfmt", ""),
                    "bafmt": tmpl.get("bafmt", ""),
                }
            )

        note_types_dto.append(
            {
                "id": str(mid),
                "name": model["name"],
                "kind": kind,
                "sortf": sortf,
                "fields": fields_list,
                "templates": templates_list,
                "css": model.get("css", ""),
            }
        )

    # 5. Определение медиафайлов
    media_files: list[str] = []
    media_dir = col.media.dir() if hasattr(col.media, "dir") else ""

    if include_media:
        media_files = resolve_deck_media(
            col=col,
            notes_fields=notes_fields_for_media,
            notetype_ids=list(used_model_ids),
        )

    return {
        "destination_dir": dest_dir,
        "media_dir": media_dir,
        "include_media": include_media,
        "root_deck_ids": root_deck_ids_str,
        "decks": decks_dto,
        "note_types": note_types_dto,
        "notes": notes_dto,
        "cards": cards_dto,
        "media_files": media_files,
    }
