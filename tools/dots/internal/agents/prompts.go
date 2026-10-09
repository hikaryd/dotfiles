package agents

const mrPrompt = `
# РОЛЬ: Ты — опытный AI-ассистент, специализирующийся на анализе кода и создании Merge Request.
# ЗАДАЧА: На основе предоставленного Git diff-а создай информативный заголовок и структурированное, подробное описание Merge Request. Ответь на русском языке.

# ВХОД (diff для анализа):
{diff}

# ИНСТРУКЦИИ:
## 1. Заголовок:
* Краткость: Не более 72 символов.
* Ясность: Отрази основное изменение или цель.
* Формат (Conventional Commits): feat:, fix:, refactor:, perf:, docs:, test:, chore:.
## 2. Описание:
* Что было сделано: Краткое резюме и перечень всех типов изменений: feat, fix, refactor, perf, docs, test, chore, style.
* Зачем это было сделано: Мотивация или бизнес-требование.
* Как проверить (необязательно): Шаги для тестирования.
* Дополнительно (необязательно): Риски, открытые вопросы.

# ФОРМАТ ВЫВОДА (строго соблюдай):
Title: <сгенерированный заголовок>
Description: <сгенерированное описание>
`
const commitPrompt = `
Generate an English commit message (title + body) for the staged Git diff below,
following the Conventional Commits format.

INPUT (staged diff):
{diff}

TITLE RULES:
* English, imperative mood, no trailing period
* Format: type(optional-scope): summary
* Types: feat, fix, docs, style, refactor, perf, test, build, ci, chore
* Maximum 72 characters
* Capture the single most important change of the whole diff

BODY RULES:
* Write the body as a bulleted list ("- " per line) of LOGICAL changes, not a
  file-by-file or line-by-line changelog
* Group all edits that serve one purpose into a SINGLE bullet, even if they span
  several files, functions or layers (e.g. backend + frontend + tests)
* Aim for a small number of bullets — typically 2 to 5; merge related work
  rather than splitting it. Most commits need 2-4 bullets, not one per file
* Each bullet describes the change at the level of behavior or intent (WHAT it
  does and WHY), not implementation mechanics
* Do NOT enumerate every new symbol, function or file; name a specific
  file/class only when it genuinely helps a reader, not by default
* Cover every meaningful area of work, but as themes — never omit a real
  feature, yet never inflate the list with granular sub-steps
* Skip pure noise (whitespace/formatting only) unless that is the entire change
* Keep each line under ~100 characters
* Only describe what is actually present in the diff — never invent changes

OUTPUT FORMAT (title, then one blank line, then the body):
<title>

- <logical change 1>
- <logical change 2>
- ...
`
const refactorPrompt = "Проанализируй этот текст, найди и исправь ошибки. Верни только исправленный текст без комментариев.\n\n{text}"
