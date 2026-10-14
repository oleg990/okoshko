# Модель данных

Черновик сущностей для MVP. Все моменты времени хранятся в UTC (`timestamptz`), локальное время мастера только в шаблонах расписания.

## Диаграмма

```mermaid
erDiagram
    ACCOUNT ||--o{ AUTH_IDENTITY : "способы входа"
    ACCOUNT ||--|| PERFORMER : "оказывает услуги"
    ACCOUNT ||--o{ NOTIFICATION_CHANNEL : "каналы мастера"
    PERFORMER ||--o{ SERVICE : "услуги"
    PERFORMER ||--o{ WORKING_HOURS : "недельный шаблон"
    PERFORMER ||--o{ SCHEDULE_EXCEPTION : "исключения"
    PERFORMER ||--o{ CLIENT : "клиенты мастера"
    PERFORMER ||--o{ BOOKING : "записи"
    CLIENT ||--o{ BOOKING : "записи клиента"
    CLIENT ||--o{ NOTIFICATION_CHANNEL : "каналы клиента"
    SERVICE ||--o{ BOOKING : "на какую услугу"
    BOOKING ||--o{ BOOKING_STATUS_HISTORY : "история статусов"
    BOOKING ||--o{ SCHEDULED_NOTIFICATION : "напоминания"
    ACCOUNT ||--o{ ONE_TIME_TOKEN : "вход"
    BOOKING ||--o{ ONE_TIME_TOKEN : "привязка Telegram"

    ACCOUNT {
        uuid id PK
        text role "master | admin"
        text status "active | blocked"
        timestamptz created_at
    }
    AUTH_IDENTITY {
        uuid id PK
        uuid account_id FK
        text provider "telegram | vk | email"
        text external_id
    }
    PERFORMER {
        uuid id PK
        uuid account_id FK
        text slug "уникальный"
        text display_name
        text description
        text avatar_path
        text timezone "Europe/Moscow"
        text confirmation_mode "auto | manual"
        int slot_step_minutes "15 | 30"
    }
    SERVICE {
        uuid id PK
        uuid performer_id FK
        text name
        int duration_minutes
        int buffer_after_minutes
        int price
        bool is_active
    }
    WORKING_HOURS {
        uuid id PK
        uuid performer_id FK
        int weekday "1..7"
        time start_time "локальное"
        time end_time "локальное"
    }
    SCHEDULE_EXCEPTION {
        uuid id PK
        uuid performer_id FK
        date date
        bool is_day_off
        time start_time "null если выходной"
        time end_time "null если выходной"
    }
    CLIENT {
        uuid id PK
        uuid performer_id FK
        text phone "нормализованный +7XXXXXXXXXX"
        text name "последнее введённое"
        uuid account_id "null до появления аккаунтов клиентов"
    }
    BOOKING {
        uuid id PK
        uuid performer_id FK
        uuid client_id FK
        uuid service_id FK
        text status
        timestamptz start_at
        timestamptz end_at "start + длительность + буфер"
        text client_name_at_booking
        text service_name_at_booking
        int duration_minutes_at_booking
        int buffer_minutes_at_booking
        int price_at_booking
        text manage_token_hash
        timestamptz confirm_until
    }
    BOOKING_STATUS_HISTORY {
        uuid id PK
        uuid booking_id FK
        text from_status
        text to_status
        text changed_by "client | master | system"
        timestamptz changed_at
    }
    NOTIFICATION_CHANNEL {
        uuid id PK
        uuid account_id FK "ровно одно из двух"
        uuid client_id FK "ровно одно из двух"
        text type "telegram | vk"
        text address "chat id"
    }
    SCHEDULED_NOTIFICATION {
        uuid id PK
        uuid booking_id FK
        text kind "reminder_24h | reminder_2h | ..."
        timestamptz send_at
        text status "pending | sent | failed | cancelled"
    }
    ONE_TIME_TOKEN {
        uuid id PK
        text purpose "login | link_telegram"
        text token_hash
        timestamptz expires_at
        timestamptz used_at
    }
```

## Решения, которые не видно на диаграмме

### Запись хранит снимок услуги
В записи сохраняются название, длительность, буфер и цена **на момент записи**. Ссылка `service_id` остаётся для статистики «какая услуга популярнее», но деньги и время берутся из снимка. Иначе повышение цены с 3000 до 3500 изменило бы прошлые записи, а смена длительности сдвинула бы `end_at` у уже существующих.

### Клиент уникален внутри мастера
Ключ: `(performer_id, phone)`, телефон нормализуется к единому формату. Один человек, записавшийся к двум мастерам, это две строки, и мастера не видят данных друг друга.
- Имя в записи хранится как введено (`client_name_at_booking`), в карточке клиента обновляется последним значением. Одинаковый телефон и другое имя не создают второго клиента (в семье один номер на двоих).
- Телефон никем не подтверждается, поэтому он контакт, а не идентификатор личности. Доступ к записи даёт только токен.
- В будущем `account_id` у клиента заполняется после входа через Telegram. Аккаунт связывает несколько клиентских строк у разных мастеров, телефон для этого не используется.

### Канал уведомлений: один владелец
У канала заполнено ровно одно из `account_id` и `client_id` (CHECK-ограничение). Это сохраняет целостность внешних ключей, в отличие от пары `owner_type` + `owner_id`.

### Статусы записи
`pending`, `confirmed`, `declined`, `cancelled_by_client`, `cancelled_by_master`, `expired`, `completed`.
Активные (занимают время): `pending`, `confirmed`. Каждый переход пишется в историю статусов.

### Ограничения на уровне БД
- `auth_identity`: уникальная пара `(provider, external_id)`.
- `performer`: уникальный `slug`, уникальный `account_id`.
- `client`: уникальная пара `(performer_id, phone)`.
- `schedule_exception`: уникальная пара `(performer_id, date)`.
- `booking`: exclusion constraint, запрещающий пересечение интервалов `[start_at, end_at)` у одного исполнителя среди активных записей (`EXCLUDE USING gist`, потребуется расширение `btree_gist`).
- Индексы: `booking (performer_id, start_at)`, `scheduled_notification (status, send_at)`.

### Расчёт свободного времени
1. Берём шаблон на день недели.
2. Применяем исключение на дату (оно сильнее шаблона).
3. Переводим локальные часы в UTC по часовому поясу мастера.
4. Вычитаем интервалы активных записей.
5. Предлагаем начала записи с шагом `slot_step_minutes`, если помещается `длительность + буфер`.

## Открытые вопросы
- Нужны ли «особые часы» в исключениях в MVP или хватит только выходных?
- Как показывать клиенту запись, у которой мастер сменил услугу (удалил или деактивировал)?
