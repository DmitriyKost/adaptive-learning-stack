# auth-service

Сервис пользователей и авторизации. Хранит только учетные записи и refresh-токены. Информация о прогрессе, попытках, навыках и траектории пользователя должна находиться в отдельном сервисе `task-progress`.

## API

- `POST /auth/register`
- `POST /auth/login`
- `POST /auth/refresh`
- `POST /auth/logout`
- `GET /users/me`
- `GET /users/{id}`
- `GET /health`

## Переменные окружения

```env
ENV=local
HTTP_ADDR=:8080
DATABASE_URL=postgres://auth:auth@localhost:5432/auth?sslmode=disable
JWT_SECRET=change-me-in-production
ACCESS_TOKEN_TTL=15m
REFRESH_TOKEN_TTL=720h
BCRYPT_COST=12
```

## Миграции

Пример запуска через `golang-migrate`:

```bash
migrate -path ./migrations -database "$DATABASE_URL" up
```

## Запуск

```bash
go mod tidy
go run ./main.go
```

## Пример регистрации

```bash
curl -X POST http://localhost:8080/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"student@example.com","password":"strongpass123"}'
```

## Пример запроса текущего пользователя

```bash
curl http://localhost:8080/users/me \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

## Замечания по интеграции

- В JWT помещаются только `user_id`, `role`, `iat`, `exp`, `typ`.
- API Gateway может валидировать access token локально через общий `JWT_SECRET` или проксировать запросы в этот сервис.
- `task-progress` должен использовать `user_id` из JWT / заголовка `X-User-ID`, но не должен обращаться к auth-service за прогрессом.
