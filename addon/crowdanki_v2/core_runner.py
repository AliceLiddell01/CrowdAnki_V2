"""Служба запуска нативного Go-ядра CrowdAnki V2."""

from __future__ import annotations

import json
import platform
import subprocess
from pathlib import Path


class CoreExecutionError(Exception):
    """Исключение при ошибке исполнения нативного ядра."""


def get_core_binary_path() -> Path:
    """Возвращает путь к скомпилированному исполняемому файлу ядра."""
    base_dir = Path(__file__).resolve().parent
    bin_dir = base_dir / "bin"

    system = platform.system().lower()

    if system == "windows":
        exe_name = "crowdanki-core.exe"
    else:
        exe_name = "crowdanki-core"

    candidate = bin_dir / exe_name
    if candidate.exists():
        return candidate

    # Также проверим путь при локальной разработке в корне репозитория (core/cmd/crowdanki-core)
    repo_root = base_dir.parent.parent
    dev_candidate = repo_root / "dist" / "crowdanki-core.exe"
    if dev_candidate.exists():
        return dev_candidate

    raise FileNotFoundError(
        f"Не найден исполняемый файл ядра CrowdAnki V2 ({exe_name}) по пути {candidate}. "
        f"Выполните сборку аддона перед запуском."
    )


def run_core_export(request_payload: dict) -> dict:
    """Запускает ядро CrowdAnki V2 с подкомандой export и передает входные данные через stdin."""
    core_path = get_core_binary_path()

    # Обеспечиваем права на выполнение в Unix-системах
    if platform.system().lower() != "windows":
        try:
            core_path.chmod(0o755)
        except Exception:
            pass

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
        result = json.loads(stdout_bytes.decode("utf-8"))
        return result
    except Exception as err:
        stdout_preview = stdout_bytes[:500].decode("utf-8", errors="replace")
        raise CoreExecutionError(
            f"Не удалось разобрать ответ Go-ядра: {err}. Вывод: {stdout_preview}"
        ) from err
