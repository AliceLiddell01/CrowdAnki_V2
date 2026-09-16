"""Сбор снимка текущего состояния коллекции Anki для планировщика импорта CrowdAnki V2."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from anki.collection import Collection


def is_deck_name_in_scope(deck_name: str, root_deck_names: list[str]) -> bool:
    """Проверяет, входит ли колода с данным именем в управляемый import scope."""
    for root in root_deck_names:
        if deck_name == root or deck_name.startswith(f"{root}::"):
            return True
    return False


def collect_dest_snapshot(
    col: Collection,
    root_deck_names: list[str],
) -> dict[str, Any]:
    """Собирает снимок состояния целевой коллекции Anki для указанных корневых колод.

    Снимок включает:
    1. Все существующие колоды коллекции (для точного определения пути и ID).
    2. Все заметки, имеющие хотя бы одну карточку внутри управляемого import scope.
    3. ВСЕ карточки этих заметок (включая находящиеся вне scope колод) с флагом in_scope.
       Это критически важно для гарантии Out-of-scope card protection в ядре.
    4. Описания типов заметок, задействованных в собранных заметках.

    Операция использует только публичный API Anki и не обращается напрямую к SQLite.
    """
    # 1. Сбор колод
    all_deck_entries = col.decks.all_names_and_ids()
    decks_dto: list[dict[str, Any]] = []
    deck_id_to_name: dict[int, str] = {}

    for entry in all_deck_entries:
        deck_dict = col.decks.get(entry.id)
        if not deck_dict:
            continue
        name = deck_dict["name"]
        deck_id_to_name[entry.id] = name
        decks_dto.append(
            {
                "id": str(entry.id),
                "name": name,
                "description": deck_dict.get("desc", ""),
            }
        )

    # 2. Поиск карточек и заметок внутри управляемого scope
    scoped_card_ids: set[int] = set()
    for root_name in root_deck_names:
        # Запрос "deck:Root" в Anki включает саму колоду и все ее подколоды
        cids = col.find_cards(f'"deck:{root_name}"')
        scoped_card_ids.update(cids)

    scoped_note_ids: set[int] = set()
    for cid in scoped_card_ids:
        card = col.get_card(cid)
        scoped_note_ids.add(card.nid)

    # 3. Сбор заметок и ВСЕХ их карточек (как в scope, так и out-of-scope)
    notes_dto: list[dict[str, Any]] = []
    cards_dto: list[dict[str, Any]] = []
    used_model_ids: set[int] = set()

    for nid in sorted(scoped_note_ids):
        note = col.get_note(nid)
        guid = note.guid
        mid = note.mid
        used_model_ids.add(mid)

        model = col.models.get(mid)
        model_name = model["name"] if model else ""

        notes_dto.append(
            {
                "id": str(nid),
                "guid": guid,
                "note_type": model_name,
                "note_type_id": str(mid),
                "fields": dict(note.items()),
                "tags": sorted(note.tags),
            }
        )

        # Получаем ВСЕ карточки этой заметки для отслеживания out-of-scope cards
        note_card_ids = col.card_ids_of_note(nid)
        for cid in note_card_ids:
            c = col.get_card(cid)
            card_deck_name = deck_id_to_name.get(c.did, "")
            if not card_deck_name:
                deck_dict = col.decks.get(c.did)
                card_deck_name = deck_dict["name"] if deck_dict else ""
                deck_id_to_name[c.did] = card_deck_name

            in_scope = is_deck_name_in_scope(card_deck_name, root_deck_names)
            cards_dto.append(
                {
                    "id": str(c.id),
                    "note_guid": guid,
                    "ord": c.ord,
                    "deck_name": card_deck_name,
                    "in_scope": in_scope,
                }
            )

    # 4. Сбор спецификаций используемых типов заметок
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

    return {
        "decks": decks_dto,
        "note_types": note_types_dto,
        "notes": notes_dto,
        "cards": cards_dto,
    }
