# СЕВЕР — барбершоп

Сайт онлайн-записи в барбершоп. Написан на чистом Go (net/http + html/template + embed).

## Чем отличается от типичного лендинга

- **Тёмная индустриальная тема** с янтарным акцентом.
- **Отдельная страница записи** с пошаговой формой.
- **Живые слоты** через API.
- **Мои записи** по номеру телефона.
- **Telegram**: уведомление владельцу о новой записи.

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

При новой записи сайт шлёт уведомление владельцу.

1. Создай бота у [@BotFather](https://t.me/BotFather) → получи токен.
2. Напиши боту `/start`, затем узнай свой `chat_id` через `@userinfobot` или `https://api.telegram.org/bot<TOKEN>/getUpdates`.
3. В Render → Environment добавь:

```
TELEGRAM_BOT_TOKEN=123456:ABC...
TELEGRAM_CHAT_ID=123456789
TELEGRAM_BOT_USERNAME=sever_barber_bot
```

`TELEGRAM_BOT_USERNAME` — без `@`, для ссылки на сайте.
