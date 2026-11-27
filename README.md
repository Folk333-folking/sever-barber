# СЕВЕР — барбершоп

Сайт онлайн-записи в барбершоп. Написан на чистом Go (net/http + html/template + embed).

## Чем отличается от типичного лендинга

- **Тёмная индустриальная тема** с янтарным акцентом.
- **Отдельная страница записи** с пошаговой формой.
- **Живые слоты** через API.
- **Мои записи** по номеру телефона.
- **Telegram**: уведомления + запись через бота.

## Запуск локально

```bash
go run .
# http://localhost:8080
```

## Деплой на Render

1. Web Service → этот репозиторий.
2. Build: `go build -o sever .`
3. Start: `./sever`
4. Или Dockerfile.

## Telegram

При новой записи (с сайта или из бота) владелец получает уведомление.

1. Создай бота у [@BotFather](https://t.me/BotFather) → токен.
2. Напиши боту `/start`, узнай `chat_id` через `@userinfobot` или getUpdates.
3. В Render → Environment:

```
TELEGRAM_BOT_TOKEN=123456:ABC...
TELEGRAM_CHAT_ID=123456789
TELEGRAM_BOT_USERNAME=SeverBarber29Bot
WEBHOOK_BASE_URL=https://sever-barber.onrender.com
```

`TELEGRAM_BOT_USERNAME` — без `@`, для ссылки на сайте.

Для записи **через бота** добавь:

```
WEBHOOK_BASE_URL=https://sever-barber.onrender.com
```

После деплоя бот сам зарегистрирует webhook. Клиент пишет `/start` → выбирает мастера, услугу, день, время → имя и телефон.
