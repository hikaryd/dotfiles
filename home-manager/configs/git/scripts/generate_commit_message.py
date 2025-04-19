import argparse
import json
import os
import subprocess
import urllib.error
import urllib.parse
import urllib.request

COMMIT_MESSAGE_PROMPT = """
Создайте сообщение коммита на русском языке на основе следующего diff:

```
{diff}
```

Сообщение коммита должно состоять из двух частей:

1. Заголовок (Title):
   - Краткое описание основных изменений
   - Не должен превышать 50 символов
   - Использовать глагол в повелительном наклонении (например, "добавить", "исправить", "обновить")

2. Детальное описание:
   - Каждый тип изменений на отдельной строке
   - Формат каждой строки: <тип>(область): <описание>
   - Быть информативным, но кратким
   - Использовать глаголы в прошедшем времени (например, "добавлено", "исправлено", "обновлено")
   - Каждая строка не должна превышать 72 символа в длину

Типы коммитов:
- feat: новая функциональность
- fix: исправление ошибки
- docs: изменения в документации
- style: изменения форматирования, отступов и т.д. (не влияющие на код)
- refactor: рефакторинг кода
- perf: улучшения производительности
- test: добавление или изменение тестов
- build: изменения в системе сборки или внешних зависимостях
- ci: изменения в конфигурации CI/CD
- chore: другие изменения, не затрагивающие код или тесты

Если в diff присутствуют изменения, относящиеся к нескольким типам, включите их все в детальное описание, каждый на отдельной строке.
Пожалуйста, создайте сообщение коммита, соответствующее этим критериям и основанное на предоставленном diff.

Пример:
feat(fix): исправление ошибки ...

Формат ответа должен быть следующим:
<заголовок коммита>

<детальное описание>
"""


def get_api_key(api_type):
    if api_type == 'openai':
        return os.getenv('OPENAI_API_KEY')
    elif api_type == 'claude':
        return os.getenv('ANTHROPIC_API_KEY')
    elif api_type == 'operouter':
        return os.getenv('OPENROUTER_API_KEY')
    else:
        raise ValueError(f'Неизвестный тип API: {api_type}')


def get_git_diff():
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
        text=True,
    )
    return result.stdout


def make_request(url, headers, data, proxy_host='127.0.0.1', proxy_port='2080'):
    proxy_handler = urllib.request.ProxyHandler(
        # {
        #     'http': f'http://{proxy_host}:{proxy_port}',
        #     'https': f'http://{proxy_host}:{proxy_port}',
        # }
    )
    opener = urllib.request.build_opener(proxy_handler)
    urllib.request.install_opener(opener)

    req = urllib.request.Request(
        url,
        data=json.dumps(data).encode('utf-8'),
        headers=headers,
        method='POST',
    )

    try:
        with urllib.request.urlopen(req) as response:
            return json.loads(response.read().decode('utf-8'))
    except urllib.error.HTTPError as e:
        print(f'Ошибка HTTP: {e.code} {e.reason}')
        print(e.read().decode('utf-8'))
        return None
    except urllib.error.URLError as e:
        print(f'Ошибка URL: {e.reason}')
        return None


def generate_commit_message_claude(diff, api_key):
    headers = {
        'Content-Type': 'application/json',
        'x-api-key': api_key,
        'anthropic-version': '2023-06-01',
    }
    data = {
        'messages': [
            {
                'role': 'user',
                'content': COMMIT_MESSAGE_PROMPT.format(diff=diff),
            }
        ],
        'max_tokens': 3000,
        'model': 'claude-3-5-sonnet-20241022',
    }

    response = make_request(
        'https://api.anthropic.com/v1/messages', headers=headers, data=data
    )

    if response:
        return response.get('content', [{}])[0].get('text', '').strip()
    return ''


def generate_commit_message_openrouter(diff, api_key):
    headers = {
        'Authorization': f'Bearer {api_key}',
    }
    data = {
        'messages': [
            {
                'role': 'user',
                'content': COMMIT_MESSAGE_PROMPT.format(diff=diff),
            }
        ],
        'model': 'google/gemini-2.5-flash-preview:thinking',
    }

    response = make_request(
        'https://openrouter.ai/api/v1/chat/completions',
        headers=headers,
        data=data,
    )

    if response:
        try:
            return (
                response.get('choices', [{}])[0]
                .get('message', '')
                .get('content', '')
                .strip()
            )
        except Exception as e:
            print(f'Ошибка при генерации сообщения коммита: {e}.\n\n{response}')
    return ''


def generate_commit_message_openai(diff, api_key):
    headers = {
        'Content-Type': 'application/json',
        'Authorization': f'Bearer {api_key}',
    }
    data = {
        'model': 'gpt-4',
        'messages': [
            {
                'role': 'system',
                'content': 'Вы - помощник по генерации сообщений коммитов.',
            },
            {
                'role': 'user',
                'content': COMMIT_MESSAGE_PROMPT.format(diff=diff),
            },
        ],
        'max_tokens': 3000,
    }

    response = make_request(
        'https://api.openai.com/v1/chat/completions',
        headers=headers,
        data=data,
    )

    if response:
        return response['choices'][0]['message']['content'].strip()
    return ''


def format_commit_message(message):
    lines = message.split('\n')
    title = lines[0].replace('TITLE: ', '')
    body = '\n'.join(lines[2:])
    return f'{title}\n\n{body}'


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Генератор сообщений коммитов')
    parser.add_argument(
        '--openai',
        action='store_true',
        help='Использовать OpenAI API вместо Claude',
    )
    parser.add_argument(
        '--operouter',
        action='store_true',
        help='Использовать OpenRouter API',
    )
    args = parser.parse_args()

    diff = get_git_diff()
    if diff.strip() == '':
        print('Нет изменений для коммита.')
    else:
        if args.openai:
            api_key = get_api_key('openai')
            commit_message = generate_commit_message_openai(diff, api_key)
        elif args.operouter:
            api_key = get_api_key('operouter')
            commit_message = generate_commit_message_openrouter(diff, api_key)
        else:
            api_key = get_api_key('claude')
            commit_message = generate_commit_message_claude(diff, api_key)

        formatted_message = format_commit_message(commit_message)
        print(formatted_message)
