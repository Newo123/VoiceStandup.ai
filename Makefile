COMPOSE := docker compose
ENV_FILE ?= .env
COMPOSE_CMD := $(COMPOSE) --env-file $(ENV_FILE)
TOOLS_COMPOSE_CMD := $(COMPOSE_CMD) --profile tools

.DEFAULT_GOAL := help

.PHONY: help env-check env-init config build up infra-up down down-volumes ps logs \
	migrate-up migrate-down migrate-redo migrate-status migrate-version migrate-create

help: ## Показать список команд
	@echo "Доступные команды:"
	@echo "  make env-init                  Создать .env из .env.example"
	@echo "  make config                    Проверить Docker Compose"
	@echo "  make build                     Собрать образ Goose migrator"
	@echo "  make up                        Запустить инфраструктуру и применить миграции"
	@echo "  make infra-up                  Запустить только PostgreSQL и Redis"
	@echo "  make down                      Остановить и удалить контейнеры"
	@echo "  make down-volumes              Удалить контейнеры и локальные данные"
	@echo "  make ps                        Показать состояние сервисов"
	@echo "  make logs [service=postgres]   Показать логи"
	@echo "  make migrate-up                Применить новые миграции"
	@echo "  make migrate-down              Откатить последнюю миграцию"
	@echo "  make migrate-redo              Откатить и применить последнюю миграцию"
	@echo "  make migrate-status            Показать состояние миграций"
	@echo "  make migrate-version           Показать текущую версию миграций"
	@echo "  make migrate-create name=...   Создать новую SQL-миграцию"

env-check:
	@test -f "$(ENV_FILE)" || (echo "Файл $(ENV_FILE) не найден. Выполните: make env-init" && exit 1)

env-init:
	@test ! -f .env || (echo "Файл .env уже существует" && exit 1)
	cp .env.example .env
	@echo "Создан .env. Замените значения change_me перед запуском."

config: env-check ## Проверить итоговую конфигурацию Compose
	$(TOOLS_COMPOSE_CMD) config --quiet

build: env-check ## Собрать образ Goose migrator
	$(TOOLS_COMPOSE_CMD) build migrator

up: infra-up ## Запустить инфраструктуру и применить миграции
	$(MAKE) migrate-up ENV_FILE="$(ENV_FILE)"

infra-up: env-check ## Запустить PostgreSQL и Redis
	$(COMPOSE_CMD) up -d postgres redis

down: env-check ## Остановить сервисы
	$(TOOLS_COMPOSE_CMD) down

down-volumes: env-check ## Удалить сервисы и именованные тома с данными
	$(TOOLS_COMPOSE_CMD) down --volumes

ps: env-check ## Показать состояние сервисов
	$(TOOLS_COMPOSE_CMD) ps --all

logs: env-check ## Показать логи всех сервисов или service=<имя>
	$(TOOLS_COMPOSE_CMD) logs --follow $(service)

migrate-up: env-check ## Применить все новые миграции
	$(TOOLS_COMPOSE_CMD) run --rm migrator up

migrate-down: env-check ## Откатить последнюю миграцию
	$(TOOLS_COMPOSE_CMD) run --rm migrator down

migrate-redo: env-check ## Откатить и повторно применить последнюю миграцию
	$(TOOLS_COMPOSE_CMD) run --rm migrator redo

migrate-status: env-check ## Показать состояние миграций
	$(TOOLS_COMPOSE_CMD) run --rm migrator status

migrate-version: env-check ## Показать текущую версию миграций
	$(TOOLS_COMPOSE_CMD) run --rm migrator version

migrate-create: env-check ## Создать миграцию: make migrate-create name=add_feature
	@test -n "$(name)" || (echo "Укажите имя: make migrate-create name=add_feature" && exit 1)
	$(TOOLS_COMPOSE_CMD) run --rm \
		--volume ./migrations:/migrations \
		migrator -dir /migrations create "$(name)" sql
