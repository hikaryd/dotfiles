PYTHON ?= python3
.DEFAULT_GOAL := help
.PHONY: help install dist

help:
	@cat USAGE

install:
	PYTHON="$(PYTHON)" bash ./install.sh

dist:
	@test "$$(git rev-parse --show-prefix)" = tools/claude-setup/ || { printf 'Запустите make из tools/claude-setup в dots.\n' >&2; exit 1; }
	@test -z "$$(git status --porcelain -- .)" || { printf 'Сначала сохраните изменения claude-setup в коммит.\n' >&2; exit 1; }
	@umask 077; mkdir -p dist; git -C ../.. archive --format=tar.gz --prefix=claude-setup/ -o tools/claude-setup/dist/claude-setup.tar.gz.new HEAD:tools/claude-setup && mv dist/claude-setup.tar.gz.new dist/claude-setup.tar.gz
	@printf 'Готово: dist/claude-setup.tar.gz (только claude-setup, без остальных dotfiles).\n'
