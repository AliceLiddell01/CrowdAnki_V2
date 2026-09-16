"""Служба запуска нативного Go-ядра CrowdAnki V2."""

from __future__ import annotations

import json
import logging
import platform
import subprocess
from pathlib import Path
from typing import Any

logger = logging.getLogger("crowdanki_v2")


class CoreExecutionError(Exception):
    """Исключение при ошибке исполнения нативного ядра."""


def get_core_binary_path() -> Path:
    """Возвращает путь к скомпилированному исполняемому файлу ядра."""
    base_dir = Path(__file__).resolve().parent
    bin_dir = base_dir / "bin"

    system = platform.system().lower()
    exe_name = "crowdanki-core.exe" if system == "windows" else "crowdanki-core"

    candidate = bin_dir / exe_name
    if candidate.exists():
        return candidate

    # Проверка пути в артефактах сборки dist/
    repo_root = base_dir.parent.parent
    dev_candidate = repo_root / "dist" / exe_name
    if dev_candidate.exists():
        return dev_candidate

    raise FileNotFoundError(
        f"Не найден исполняемый файл ядра CrowdAnki V2 ({exe_name}) по пути {candidate}. "
        f"Выполните сборку аддона перед запуском."
    )


def validate_core_result(result: Any) -> dict:
    """Проверяет структурную корректность ответа от Go-ядра."""
    if not isinstance(result, dict):
        raise CoreExecutionError(
            "Повреждённый/неожиданный ответ Go core: ответ не является объектом"
        )

    required_fields = [
        ("decks", int),
        ("notes", int),
        ("cards", int),
        ("note_types", int),
        ("media_files", int),
        ("media_bytes", int),
        ("missing_media", int),
        ("elapsed_ms", int),
        ("media_elapsed_ms", int),
    ]

    for field, expected_type in required_fields:
        if field not in result:
            raise CoreExecutionError(
                f"Повреждённый/неожиданный ответ Go core: отсутствует обязательное поле '{field}'"
            )
        val = result[field]
        if not isinstance(val, expected_type) or val < 0:
            msg = f"Повреждённый ответ Go core: '{field}' некорректно ({val})"
            raise CoreExecutionError(msg)

    return result


def validate_import_plan_result(result: Any) -> dict:
    """Проверяет структурную корректность ответа планировщика импорта от Go-ядра."""
    if not isinstance(result, dict):
        raise CoreExecutionError(
            "Повреждённый/неожиданный ответ Go core: ответ не является объектом"
        )

    if "can_apply" not in result or not isinstance(result["can_apply"], bool):
        raise CoreExecutionError(
            "Повреждённый ответ Go core: отсутствует или некорректно поле 'can_apply'"
        )

    if "summary" not in result or not isinstance(result["summary"], dict):
        raise CoreExecutionError(
            "Повреждённый ответ Go core: отсутствует или некорректно поле 'summary'"
        )

    summary = result["summary"]
    required_summary_fields = [
        "total_decks",
        "total_notes",
        "total_cards",
        "total_note_types",
        "created_decks",
        "deleted_decks",
        "created_notes",
        "updated_notes",
        "deleted_notes",
        "created_cards",
        "moved_cards",
        "deleted_cards",
        "created_note_types",
        "updated_note_types",
        "added_media",
        "same_media",
        "conflict_media",
        "missing_media",
        "total_conflicts",
    ]
    for sf in required_summary_fields:
        if sf not in summary or not isinstance(summary[sf], int) or summary[sf] < 0:
            raise CoreExecutionError(f"Повреждённый ответ Go core: summary['{sf}'] некорректно")

    for list_field in (
        "conflicts",
        "warnings",
        "deck_ops",
        "note_type_ops",
        "note_ops",
        "card_ops",
        "media_ops",
    ):
        if list_field not in result or not isinstance(result[list_field], list):
            raise CoreExecutionError(
                f"Повреждённый ответ Go core: отсутствует обязательный список '{list_field}'"
            )

    return result


def get_subprocess_popen_kwargs(system_name: str | None = None) -> dict:
    """Возвращает параметры subprocess.Popen для текущей или заданной ОС.

    На Windows подавляет появление консольного окна через флаг CREATE_NO_WINDOW.
    На других платформах (Linux, macOS) Windows-специфичные флаги не применяются.
    """
    sys_name = (system_name or platform.system()).lower()
    kwargs: dict = {}
    if sys_name == "windows":
        create_no_window = getattr(subprocess, "CREATE_NO_WINDOW", 0x08000000)
        kwargs["creationflags"] = create_no_window
    return kwargs


def format_process_failure_message(returncode: int, stderr_text: str) -> str:
    """Формирует понятное пользователю сообщение об ошибке завершения процесса."""
    raw_uint32 = returncode & 0xFFFFFFFF
    # 0xC000013A = 3221225786 = STATUS_CONTROL_C_EXIT
    if raw_uint32 == 0xC000013A or returncode in (-2, 130):
        return "Процесс экспорта был прерван пользователем или закрыт системой (0xC000013A)."
    if returncode in (-15, -9, 143, 137):
        return "Процесс экспорта был принудительно остановлен (сигнал завершения)."

    if stderr_text:
        return f"Ошибка при выполнении экспорта: {stderr_text}"
    return f"Go-ядро непредвиденно завершилось (код возврата: {returncode})"


def run_core_export(request_payload: dict) -> dict:
    """Запускает ядро CrowdAnki V2 с подкомандой export и передает входные данные через stdin."""
    core_path = get_core_binary_path()

    # Обеспечиваем права на выполнение в Unix-системах
    if platform.system().lower() != "windows":
        try:
            core_path.chmod(0o755)
        except Exception as err:
            logger.warning("Не удалось изменить права на исполняемый файл %s: %s", core_path, err)

    input_json = json.dumps(request_payload, ensure_ascii=False).encode("utf-8")
    popen_kwargs = get_subprocess_popen_kwargs()

    try:
        proc = subprocess.Popen(
            [str(core_path), "export"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            **popen_kwargs,
        )
    except OSError as err:
        raise CoreExecutionError(
            f"Не удалось запустить процесс Go-ядра CrowdAnki V2 ({core_path}): {err}"
        ) from err

    stdout_bytes, stderr_bytes = proc.communicate(input=input_json)

    if proc.returncode != 0:
        err_msg = stderr_bytes.decode("utf-8", errors="replace").strip()
        raw_code = proc.returncode
        raw_hex = f"0x{raw_code & 0xFFFFFFFF:08X}"
        logger.error(
            "Процесс Go-ядра завершился с кодом %s (%s). Stderr: %s",
            raw_code,
            raw_hex,
            err_msg,
        )
        user_msg = format_process_failure_message(raw_code, err_msg)
        raise CoreExecutionError(user_msg)

    try:
        raw_result = json.loads(stdout_bytes.decode("utf-8"))
    except Exception as err:
        stdout_preview = stdout_bytes[:500].decode("utf-8", errors="replace")
        raise CoreExecutionError(
            f"Не удалось разобрать ответ Go-ядра: {err}. Вывод: {stdout_preview}"
        ) from err

    return validate_core_result(raw_result)


def run_core_import_plan(request_payload: dict) -> dict:
    """Запускает ядро CrowdAnki V2 с подкомандой plan-import для построения ImportPlan."""
    core_path = get_core_binary_path()

    if platform.system().lower() != "windows":
        try:
            core_path.chmod(0o755)
        except Exception as err:
            logger.warning("Не удалось изменить права на исполняемый файл %s: %s", core_path, err)

    input_json = json.dumps(request_payload, ensure_ascii=False).encode("utf-8")
    popen_kwargs = get_subprocess_popen_kwargs()

    try:
        proc = subprocess.Popen(
            [str(core_path), "plan-import"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            **popen_kwargs,
        )
    except OSError as err:
        raise CoreExecutionError(
            f"Не удалось запустить процесс Go-ядра CrowdAnki V2 ({core_path}): {err}"
        ) from err

    stdout_bytes, stderr_bytes = proc.communicate(input=input_json)

    if proc.returncode != 0:
        err_msg = stderr_bytes.decode("utf-8", errors="replace").strip()
        raw_code = proc.returncode
        raw_hex = f"0x{raw_code & 0xFFFFFFFF:08X}"
        logger.error(
            "Процесс Go-ядра (plan-import) завершился с кодом %s (%s). Stderr: %s",
            raw_code,
            raw_hex,
            err_msg,
        )
        user_msg = format_process_failure_message(raw_code, err_msg)
        raise CoreExecutionError(user_msg)

    try:
        raw_result = json.loads(stdout_bytes.decode("utf-8"))
    except Exception as err:
        stdout_preview = stdout_bytes[:500].decode("utf-8", errors="replace")
        raise CoreExecutionError(
            f"Не удалось разобрать ответ Go-ядра при планировании импорта: {err}. "
            f"Вывод: {stdout_preview}"
        ) from err

    return validate_import_plan_result(raw_result)
