<img src="./logo_light.svg#gh-light-mode-only" alt="logo" width="400" />
<img src="./logo_dark.svg#gh-dark-mode-only" alt="logo" width="400" />

# Прокси telegram

Тот самый безопасный SOCKS5 прокси.

[proxi.soaska.ru](https://proxi.soaska.ru)

---

## HTTP API (кратко)

### Публичные эндпойнты
- `GET /api/stats/public` — агрегированная статистика (аптайм, подключения, трафик, топ стран).
- `GET /api/speedtest/latest` — последний результат speedtest.
- `GET /api/speedtest/history` — история последних измерений скорости.
- `POST /api/speedtest/trigger` — запуск нового speedtest (параметр `source`, ответ `accepted`).

## Telegram бот (для админов)

- `/start` — краткое приветствие + основные команды (статистика, трафик, speedtest, info).
- `/help` — полный список команд, подсказки и ограничения.
- `/stats`, `/traffic`, `/countries`, `/top`, `/recent`, `/today`, `/week`, `/peak`, `/compare`.
- `/speedtest`, `/speedtest_result`, `/search <country>`, `/export`, `/status`, `/health`, `/info`, `/ip`.

### Приватные эндпойнты
*(требуется заголовок `Authorization: Bearer <API_KEY>`)*

- `GET /api/admin/connections` — история подключений с фильтрами (`country`, `client_ip`, `target`, `since`, `until`, `limit`, `offset`) и статистикой суммарного трафика.
- `GET /api/admin/stats/traffic` — свод по трафику (download/upload, средние значения).
- `GET /api/admin/stats/countries` — распределение по странам (параметр `limit`).
- `GET /api/admin/stats/recent` — последние завершённые подключения.
- `GET /api/admin/stats/today` — поминутная статистика за текущие сутки.
- `GET /api/admin/stats/week` — посуточная статистика за 7 дней.
- `GET /api/admin/stats/peak` — пиковые часы/дни и самая активная страна.
- `GET /api/admin/stats/compare` — сравнение “сегодня/вчера”, “эта неделя/прошлая”.
- `GET /api/admin/stats/search?country=XX` — детали по стране + последние сессии.
- `GET /api/admin/stats/export` — снапшот публичной статистики и топ стран.
- `GET /api/admin/stats/status` — состояние сервиса (аптайм, активные подключения, суммарный трафик).
- `GET /api/admin/stats/health` — здоровье подсистем (БД, сборщик, speedtest).
- `GET /api/admin/stats/info` — расширенная информация (размер БД, число стран, топ страна).

---
credit to huecker.io