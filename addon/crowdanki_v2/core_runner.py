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

    try:
        proc = subprocess.Popen(
            [str(core_path), "export"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
    except OSError as err:
        raise CoreExecutionError(
            f"Не удалось запустить процесс Go-ядра CrowdAnki V2 ({core_path}): {err}"
        ) from err

    stdout_bytes, stderr_bytes = proc.communicate(input=input_json)

    if proc.returncode != 0:
        err_msg = stderr_bytes.decode("utf-8", errors="replace").strip()
        raise CoreExecutionError(
            f"Go-ядро завершилось с ошибкой (код {proc.returncode}): {err_msg}"
        )

    try:
        raw_result = json.loads(stdout_bytes.decode("utf-8"))
    except Exception as err:
        stdout_preview = stdout_bytes[:500].decode("utf-8", errors="replace")
        raise CoreExecutionError(
            f"Не удалось разобрать ответ Go-ядра: {err}. Вывод: {stdout_preview}"
        ) from err

    return validate_core_result(raw_result)
