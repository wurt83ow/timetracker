### TimeTracker

TimeTracker — это система для отслеживания рабочего времени сотрудников, реализованная на языке Go. Проект предоставляет REST API для управления пользователями и их трудозатратами.

#### Основные возможности:

1. Получение данных пользователей с фильтрацией и пагинацией.
2. Получение трудозатрат по пользователю за период с сортировкой.
3. Управление отсчетом времени по задачам для пользователей.
4. Удаление пользователей.
5. Изменение данных пользователей.
6. Добавление новых пользователей.
7. Получение  информации через внешний API и обогащение данных пользователей. Для получения данных от API и их обогащения используется workerpool, который выполняется периодически с интервалом, заданным в настройках.  


### Решения и ограничения, принятые для реализации тестового задания

- **Автоматическое закрытие незавершенных задач**:
  Задача, которая не была закрыта пользователем в текущем дне (поле "End time" пустое), автоматически считается закрытой той же датой концом рабочего дня\*. Следовательно, для продолжения работы над задачей на следующий день, пользователь должен стартовать задачу заново. Такой подход избавит нас от необходимости регламентно создавать записи в базе, а также обрабатывать ситуации, когда регламент по аварийным причинам не работал в течение какого-то времени или была запущена тестовая копия рабочей базы, что может привести к длительному созданию записей. Кроме того, если задачи будут в рамках одного дня, будет проще работать с индексами и создавать соединения с другими таблицами, например, для тарификации времени разработчика. Пустая дата завершения = Конец рабочего дня. При необходимости можно реализовать отдельный флаг в настройках пользователя "Auto-carry over unfinished tasks" и логику для переноса задач.

- **Запрет на старт активной задачи**:
  Если пользователь пытается стартовать активную\*\* задачу, ему будет запрещено это действие с выбросом ошибки о том, что по задаче уже ведется трекинг.

- **Обработка завершения трекинга по закрытой или отсутствующей задаче**:
  Если пользователь пытается завершить трекинг по уже закрытой или отсутствующей записи, ему будет выброшена ошибка о том, что задача уже была завершена или активная запись не найдена.

---

(\*) **Конец рабочего дня**: конец рабочего дня из настроек пользователя или календарный конец дня "23:59:59".

(\*\*) Время завершения не заполнено.

#### Планируемые улучшения:

- Покрытие проекта тестами.
- Реализация более гибких возможностей для получения трудозатрат по всем пользователям.
- Разработка клиентской части на React 18+.
- Заменить "PassportNumber" на уникальный идентификатор (id) в запросах на удаление и обновление пользователя, а также в запросе для получения трудозатрат пользователя за определенный период.  

#### Конфигурация проекта

Файл `.env` содержит следующие параметры конфигурации:

```plaintext
RUN_ADDRESS=:8080
LOG_LEVEL=debug
DATABASE_URI=postgres://timetracker:example@localhost:5432/timetracker?sslmode=disable
JWT_SIGNING_KEY=test_key
CONCURRENCY=5
TASK_EXECUTION_INTERVAL=3000
USER_UPDATE_INTERVAL="5m"
DEFAULT_END_TIME="19:00"
API_SYSTEM_ADDRESS="localhost:8081"
```

- **RUN_ADDRESS**: Адрес и порт для запуска сервера (по умолчанию `:8080`).
- **LOG_LEVEL**: Уровень логирования (`debug`).
- **DATABASE_URI**: URI для подключения к базе данных PostgreSQL.
- **JWT_SIGNING_KEY**: Ключ для подписи JWT.
- **CONCURRENCY**: Количество одновременно выполняемых задач.
- **TASK_EXECUTION_INTERVAL**: Интервал выполнения задач (в миллисекундах).
- **USER_UPDATE_INTERVAL**: Интервал обновления пользователей.
- **DEFAULT_END_TIME**: Время окончания работы по умолчанию.
- **API_SYSTEM_ADDRESS**: Адрес API системы.

#### Используемые технологии:

- **Go**: Основной язык программирования.
- **PostgreSQL**: База данных для хранения информации.
- **Docker**: Контейнеризация сервиса.
- **Swagger**: Автоматическая генерация документации API.
- **JWT**: Аутентификация с использованием JSON Web Tokens.
- **Zap**: Логирование.

#### Структура проекта:

- **cmd**: Исходный код для командной строки.
- **docs**: Документация проекта.
- **internal**: Внутренние пакеты и код.
- **migrations**: Миграции базы данных.
- **.env**: Конфигурационный файл.
- **LICENSE**: Лицензия проекта.
- **README.md**: Основной файл с описанием проекта.
- **.gitignore**: Файл для игнорируемых Git файлов.
- **go.sum** и **go.mod**: Файлы для управления зависимостями Go.
- **docker-compose.yml**: Конфигурация Docker Compose.
- **postman_collection.json**: Коллекция Postman для тестирования API.

#### Быстрый старт:

1. Клонируйте репозиторий:
   ```bash
   git clone https://github.com/yourusername/timetracker.git
   ```
2. Перейдите в директорию проекта:
   ```bash
   cd timetracker
   ```
3. Создайте файл `.env` и заполните его необходимыми параметрами (можно использовать пример из вышеуказанного описания).
4. Запустите Docker Compose для создания базы данных Postgresql:
   ```bash
   docker compose up -d
   ```
5. Перейдите в каталог `cmd/timetracker` и выполните команду:
   ```bash
   go run main.go
   ```
   Эта команда выполнит миграции базы данных и запустит сервер.

### Шаги для заполнения базы данных тестовыми данными из файла insert_test_data.sql

1. Убедиться, что БД запущена
2. Убедиться, что таблицы в БД созданы
3. Перейти в каталог migrations
4. Выполнить команду:

```bash
psql postgres://timetracker:example@localhost:5432/timetracker?sslmode=disable -f insert_test_data.sql

#### Отладка:

Для отладки можно использовать файл `postman_collection.json`, который находится в корне проекта. Импортируйте этот файл в Postman, чтобы использовать готовые запросы для тестирования API.

#### REST API эндпоинты:
- **GET /users**: Получение данных пользователей с фильтрацией и пагинацией.
- **GET /users/{id}/worklog**: Получение трудозатрат по пользователю за период.
- **POST /users/{id}/tasks/start**: Начать отсчет времени по задаче.
- **POST /users/{id}/tasks/stop**: Закончить отсчет времени по задаче.
- **DELETE /users/{id}**: Удаление пользователя.
- **PUT /users/{id}**: Изменение данных пользователя.
- **POST /users**: Добавление нового пользователя.

#### Лицензия
Проект распространяется под лицензией MIT. Смотрите файл [LICENSE](./LICENSE) для получения дополнительной информации.

---

Этот README предоставляет основную информацию для начала работы с проектом TimeTracker. Вы можете расширять и обновлять его по мере необходимости, чтобы включать дополнительные детали или инструкции.
```
