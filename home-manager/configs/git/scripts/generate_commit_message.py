#!/usr/bin/env python3

import argparse
import json
import os
import subprocess
import sys
import urllib.error
import urllib.request
from typing import Optional, Dict, Any

COMMIT_MESSAGE_PROMPT = """
Ты – эксперт по написанию сообщений коммитов на русском языке, строго следующий правилам Conventional Commits.
Создать сообщение для коммита на русском языке на основе предоставленного diff.

ВХОДНЫЕ ДАННЫЕ:
diff:
```
{diff}
```

# ОБЩИЕ ТРЕБОВАНИЯ:
*   Сообщение коммита должно быть полностью на русском языке.
*   Сообщение состоит из двух обязательных частей: заголовок и детальное описание, разделенных пустой строкой.
*   Строго придерживайся типов коммитов Conventional Commits.

# 1. ЗАГОЛОВОК (Title):
*   **Содержание:** краткое, высокоуровневое описание сути изменений.
*   **Формат:** `<описание>` (область опциональна).
*   **Глагол:** формулировка в повелительном наклонении (императив).
*   **Длина:** не более 50 символов.
*   **Регистр:** начинается с заглавной буквы.
*   **Точка в конце:** не ставится.

# 2. ДЕТАЛЬНОЕ ОПИСАНИЕ (Body):
*   **Назначение:** объясняет «что» и «почему», а не «как».
*   Следует после заголовка через одну пустую строку.
*   Может содержать несколько параграфов или маркеры `<тип>(<область>): <описание>` в прошедшем времени.

# ТИПЫ КОММИТОВ:
*   `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`.

# ФОРМАТ ОТВЕТА:
<Заголовок>

<Детальное описание (если применимо)>
"""

MODEL_NAME = os.getenv("GEMINI_MODEL", "gemini-2.5-flash-preview-04-17")
API_KEY_ENV = "GOOGLE_API_KEY"


def get_api_key() -> str:
    """
    Получает ключ API из переменной окружения.

    :raises SystemExit: если переменная не установлена.
    :return: строка с API-ключом.
    """
    key = os.getenv(API_KEY_ENV)
    if not key:
        print(f"Ошибка: переменная окружения {API_KEY_ENV} не установлена.", file=sys.stderr)
        sys.exit(1)
    return key


def get_git_diff() -> str:
    """
    Извлекает разницу (diff) из индексированных изменений Git.

    :return: текст diff.
    """
    result = subprocess.run(
        [
            'git',
            '--no-pager',
            '-c',
            'delta.enable=false',
            'diff',
            '--cached',
            '--text',
        ],
        stdout=subprocess.PIPE,
        text=True
    )
    return result.stdout


def send_request(api_key: str, prompt: str) -> Optional[Dict[str, Any]]:
    """
    Отправляет запрос к Gemini Free API и возвращает ответ.

    :param api_key: API-ключ для доступа к сервису.
    :param prompt: текстовый промпт для модели.
    :return: словарь с ответом или None при ошибке.
    """
    url = (
        f"https://generativelanguage.googleapis.com/v1beta/models/"
        f"{MODEL_NAME}:generateContent?key={api_key}"
    )
    payload = {"contents": [{"parts": [{"text": prompt}]}]}
    headers = {"Content-Type": "application/json; charset=utf-8"}
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(req) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as err:
        print(f"Ошибка HTTP: {err.code} {err.reason}", file=sys.stderr)
        print(err.read().decode("utf-8"), file=sys.stderr)
    except urllib.error.URLError as err:
        print(f"Ошибка URL: {err.reason}", file=sys.stderr)
    return None


def extract_answer(response: Dict[str, Any]) -> str:
    """
    Извлекает текстовое содержание ответа модели из структуры API.

    :param response: словарь с данными ответа API.
    :return: текст ответа.
    """
    candidates = response.get("candidates")
    if candidates:
        parts = candidates[0].get("content", {}).get("parts", [])
        return "".join(p.get("text", "") for p in parts).strip()
    return ""


def format_commit_message(message: str) -> str:
    """
    Форматирует сгенерированное сообщение коммита.

    :param message: сырой текст от модели.
    :return: отформатированное сообщение.
    """
    lines = message.strip().split("\n")
    title = lines[0]
    body = "\n".join(lines[1:]).strip()
    return f"{title}\n\n{body}" if body else title


def main() -> None:
    """
    Основная функция: извлекает diff, запрашивает модель и выводит сообщение.
    """
    parser = argparse.ArgumentParser(description='Генератор сообщений коммитов с Gemini API')
    parser.add_argument(
        '--model',
        help='Имя модели Gemini (по умолчанию из GEMINI_MODEL)',
    )
    args = parser.parse_args()

    if args.model:
        global MODEL_NAME
        MODEL_NAME = args.model

    api_key = get_api_key()
    diff = get_git_diff()
    if not diff.strip():
        print('Нет изменений для коммита.')
        sys.exit(0)

    prompt = COMMIT_MESSAGE_PROMPT.format(diff=diff)
    response = send_request(api_key, prompt)
    if not response:
        print('Ошибка получения ответа от Gemini API.', file=sys.stderr)
        sys.exit(1)

    answer = extract_answer(response)
    if not answer:
        print('Пустой ответ от модели.', file=sys.stderr)
        sys.exit(1)

    commit_msg = format_commit_message(answer)
    print(commit_msg)


if __name__ == '__main__':
    main()
